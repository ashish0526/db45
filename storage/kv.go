// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// KVOptions configures the store. The database owns a directory; the caller only
// supplies its path.
type KVOptions struct {
	Dirpath string
	// LogShreshold is the MemTable size at which Compact flushes it to an
	// SSTable (0 = flush whenever non-empty). GrowthFactor drives size-tiered
	// merging of adjacent SSTable levels; <= 1 (the zero value) disables it.
	LogShreshold int
	GrowthFactor float32
}

// KV is the storage engine. The directory holds the write-ahead log (kv_log),
// the two metadata slots (meta0/meta1), and the SSTable file(s). The metadata
// names the current SSTable — the commit point of a compaction is "the metadata
// now points at the new file".
type KV struct {
	Options KVOptions
	meta    KVMetaStore
	log     Log
	mem     SortedArray
	main    []SortedFile // SSTable levels, newest (main[0]) to oldest
	version uint64
}

const logFileName = "kv_log"

// Open readies the directory: opens the metadata, replays the log into the
// MemTable, and opens the SSTable the metadata names (if any).
func (kv *KV) Open() error {
	dir := kv.Options.Dirpath
	if dir == "" {
		return errors.New("kv: Options.Dirpath is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	kv.log.FileName = filepath.Join(dir, logFileName)
	kv.meta.slots[0].FileName = filepath.Join(dir, "meta0")
	kv.meta.slots[1].FileName = filepath.Join(dir, "meta1")

	if err := kv.meta.Open(); err != nil {
		return err
	}
	kv.version = kv.meta.Get().Version

	if err := kv.log.Open(); err != nil {
		return err
	}
	kv.mem.Clear()
	for {
		var ent Entry
		eof, err := kv.log.Read(&ent)
		if err != nil {
			return err
		}
		if eof {
			break
		}
		if ent.deleted {
			kv.mem.Del(ent.key)
		} else {
			kv.mem.Set(ent.key, ent.val)
		}
	}

	return kv.openLevels()
}

// openLevels (re)opens the SSTable files the metadata names.
func (kv *KV) openLevels() error {
	for i := range kv.main {
		kv.main[i].Close()
	}
	kv.main = nil
	for _, name := range kv.meta.Get().SSTables {
		f := SortedFile{FileName: filepath.Join(kv.Options.Dirpath, name)}
		if err := f.Open(); err != nil {
			return err
		}
		kv.main = append(kv.main, f)
	}
	return nil
}

// Close closes the log, the metadata and every SSTable level.
func (kv *KV) Close() error {
	err := kv.log.Close()
	if e := kv.meta.Close(); err == nil {
		err = e
	}
	for i := range kv.main {
		if e := kv.main[i].Close(); err == nil {
			err = e
		}
	}
	return err
}

// levels returns the live levels, newest first: the MemTable then each SSTable.
func (kv *KV) levels() MergedSortedKV {
	m := MergedSortedKV{&kv.mem}
	for i := range kv.main {
		m = append(m, &kv.main[i])
	}
	return m
}

// Get returns the value for key, consulting the MemTable then the SSTable. A
// MemTable tombstone shadows the SSTable.
func (kv *KV) Get(key []byte) (val []byte, ok bool, err error) {
	switch st, v := kv.mem.state(key); st {
	case entryPresent:
		return v, true, nil
	case entryTombstone:
		return nil, false, nil
	}
	// consult SSTable levels newest-first; the first hit wins
	for i := range kv.main {
		it, err := kv.main[i].Seek(key)
		if err != nil {
			return nil, false, err
		}
		if it.Valid() && bytes.Equal(it.Key(), key) {
			if it.Deleted() {
				return nil, false, nil
			}
			return it.Val(), true, nil
		}
	}
	return nil, false, nil
}

// UpdateMode selects INSERT / UPDATE / UPSERT semantics for SetEx.
type UpdateMode int

const (
	ModeUpsert UpdateMode = 0
	ModeInsert UpdateMode = 1
	ModeUpdate UpdateMode = 2
)

// SetEx writes val under key subject to mode. It reports whether a write
// happened. Existence is checked against the merged view.
func (kv *KV) SetEx(key, val []byte, mode UpdateMode) (bool, error) {
	_, existed, err := kv.Get(key)
	if err != nil {
		return false, err
	}
	if existed && mode == ModeInsert {
		return false, nil
	}
	if !existed && mode == ModeUpdate {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, val: val}); err != nil {
		return false, err
	}
	kv.mem.Set(key, val)
	return true, nil
}

// Set is an upsert: insert or overwrite, always writing.
func (kv *KV) Set(key []byte, val []byte) (updated bool, err error) {
	return kv.SetEx(key, val, ModeUpsert)
}

// Del records a tombstone for key. deleted reports whether the key was live.
func (kv *KV) Del(key []byte) (deleted bool, err error) {
	_, existed, err := kv.Get(key)
	if err != nil {
		return false, err
	}
	if !existed {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, deleted: true}); err != nil {
		return false, err
	}
	kv.mem.Del(key)
	return true, nil
}

// Seek returns a cursor over the merged, tombstone-filtered store.
func (kv *KV) Seek(key []byte) (SortedKVIter, error) {
	it, err := kv.levels().Seek(key)
	if err != nil {
		return nil, err
	}
	return filterDeleted(it)
}

// Compact does all pending compaction work: it flushes the MemTable to a new
// main[0] SSTable, then repeatedly merges any level that has outgrown its target
// into the next, backing up on each merge so a cascade can continue. (A real
// database fires this automatically when the log crosses LogShreshold; here it
// is called explicitly.)
func (kv *KV) Compact() error {
	if kv.mem.Size() > 0 && kv.mem.Size() >= kv.Options.LogShreshold {
		if err := kv.compactLog(); err != nil {
			return err
		}
	}
	for i := 0; i+1 < len(kv.main); i++ {
		if kv.shouldMerge(i) {
			if err := kv.compactSSTable(i); err != nil {
				return err
			}
			i-- // re-check this position; the merge may have pushed i+1 over
			continue
		}
	}
	return nil
}

// compactLog converts the current MemTable to a fresh SSTable and prepends it as
// main[0]. Tombstones are kept — main[0] is the newest level, not the last, so a
// delete marker still has older copies to shadow.
func (kv *KV) compactLog() error {
	dir := kv.Options.Dirpath
	kv.version++
	name := fmt.Sprintf("sstable_%d", kv.version)

	nf := &SortedFile{FileName: filepath.Join(dir, name)}
	if err := nf.CreateFromSorted(&kv.mem); err != nil {
		nf.Close()
		return err
	}
	nf.Close()

	names := append([]string{name}, kv.meta.Get().SSTables...)
	if err := kv.meta.Set(KVMetaData{Version: kv.version, SSTables: names}); err != nil {
		return err
	}
	if err := kv.openLevels(); err != nil {
		return err
	}
	kv.mem.Clear()
	return kv.log.Truncate()
}

// shouldMerge implements a size-tiered policy: merge adjacent levels i and i+1
// when they are within GrowthFactor of the same size, so runs coalesce and level
// sizes grow geometrically. This keeps the level count O(log N) and so bounds the
// read cost; tuning GrowthFactor (and LogShreshold, the flush size) trades write
// amplification against read amplification and space — the central LSM knob.
func (kv *KV) shouldMerge(i int) bool {
	if kv.Options.GrowthFactor <= 1 {
		return false
	}
	lower := float64(kv.main[i+1].Size())
	upper := float64(kv.main[i].Size())
	return lower <= float64(kv.Options.GrowthFactor)*upper
}

// compactSSTable merges level i (newer) with level i+1 into one file. Merging
// into the final level drops tombstones physically — nothing below is left to
// shadow.
func (kv *KV) compactSSTable(i int) error {
	dir := kv.Options.Dirpath
	kv.version++
	name := fmt.Sprintf("sstable_%d", kv.version)

	var src SortedKV = MergedSortedKV{&kv.main[i], &kv.main[i+1]}
	if i+2 == len(kv.main) {
		src = NoDeletedSortedKV{SortedKV: src}
	}
	nf := &SortedFile{FileName: filepath.Join(dir, name)}
	if err := nf.CreateFromSorted(src); err != nil {
		nf.Close()
		return err
	}
	nf.Close()

	old := kv.meta.Get().SSTables
	names := make([]string, 0, len(old)-1)
	names = append(names, old[:i]...)
	names = append(names, name)
	names = append(names, old[i+2:]...)
	if err := kv.meta.Set(KVMetaData{Version: kv.version, SSTables: names}); err != nil {
		return err
	}

	superseded := []string{old[i], old[i+1]}
	if err := kv.openLevels(); err != nil {
		return err
	}
	for _, s := range superseded {
		_ = os.Remove(filepath.Join(dir, s))
	}
	return nil
}
