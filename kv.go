// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

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
}

// KV is the storage engine. The directory holds the write-ahead log (kv_log),
// the two metadata slots (meta0/meta1), and the SSTable file(s). The metadata
// names the current SSTable — the commit point of a compaction is "the metadata
// now points at the new file".
type KV struct {
	Options  KVOptions
	meta     KVMetaStore
	log      Log
	mem      SortedArray
	main     SortedFile
	mainOpen bool
	version  uint64
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

	if name := kv.meta.Get().SSTable; name != "" {
		kv.main = SortedFile{FileName: filepath.Join(dir, name)}
		if err := kv.main.Open(); err != nil {
			return err
		}
		kv.mainOpen = true
	}
	return nil
}

// Close closes the log, the metadata and the SSTable.
func (kv *KV) Close() error {
	err := kv.log.Close()
	if e := kv.meta.Close(); err == nil {
		err = e
	}
	if kv.mainOpen {
		if e := kv.main.Close(); err == nil {
			err = e
		}
	}
	return err
}

// levels returns the live levels, newest first.
func (kv *KV) levels() MergedSortedKV {
	m := MergedSortedKV{&kv.mem}
	if kv.mainOpen {
		m = append(m, &kv.main)
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
	if !kv.mainOpen {
		return nil, false, nil
	}
	it, err := kv.main.Seek(key)
	if err != nil {
		return nil, false, err
	}
	if it.Valid() && bytes.Equal(it.Key(), key) {
		return it.Val(), true, nil
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

// Compact folds the MemTable into the SSTable. It writes a new file named
// sstable_<version>, commits by recording that name in the metadata, then drops
// the MemTable, truncates the log and deletes the superseded file. A crash
// before the metadata write just leaves an orphan file the next Open ignores.
func (kv *KV) Compact() error {
	dir := kv.Options.Dirpath
	kv.version++
	name := fmt.Sprintf("sstable_%d", kv.version)

	nf := &SortedFile{FileName: filepath.Join(dir, name)}
	// The SSTable is the last (and only) level, so tombstones are dropped.
	src := NoDeletedSortedKV{SortedKV: kv.levels()}
	if err := nf.CreateFromSorted(src); err != nil {
		nf.Close()
		return err
	}
	nf.Close()

	old := kv.meta.Get().SSTable
	if err := kv.meta.Set(KVMetaData{Version: kv.version, SSTable: name}); err != nil {
		return err
	}

	if kv.mainOpen {
		kv.main.Close()
	}
	kv.main = SortedFile{FileName: filepath.Join(dir, name)}
	if err := kv.main.Open(); err != nil {
		return err
	}
	kv.mainOpen = true

	kv.mem.Clear()
	if err := kv.log.Truncate(); err != nil {
		return err
	}
	if old != "" && old != name {
		_ = os.Remove(filepath.Join(dir, old))
	}
	return nil
}
