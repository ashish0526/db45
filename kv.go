// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

import (
	"bytes"
	"errors"
	"os"
)

var errNoMainFile = errors.New("kv: no main SSTable file configured")

// KV is the storage engine. Two live levels: the in-memory MemTable (recent
// writes, mirrors the log) and one immutable on-disk SSTable (main). Reads merge
// them; Compact folds the MemTable into the SSTable and truncates the log.
type KV struct {
	log      Log
	mem      SortedArray
	main     SortedFile
	mainOpen bool
}

// Open opens the log and replays it into the MemTable, and opens the SSTable if
// one exists.
func (kv *KV) Open() error {
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

	if kv.main.FileName != "" {
		if err := kv.main.Open(); err == nil {
			kv.mainOpen = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Close closes the log and the SSTable.
func (kv *KV) Close() error {
	err := kv.log.Close()
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

// Compact merges the MemTable into the SSTable via a temp file and an atomic
// rename, then drops the MemTable and truncates the log. Ordering matters: the
// new file must be fully written and fsynced before the rename, and the log
// truncated only after the rename succeeds.
func (kv *KV) Compact() error {
	if kv.main.FileName == "" {
		return errNoMainFile
	}
	tmp := kv.main.FileName + ".compact"
	_ = os.Remove(tmp)

	nf := &SortedFile{FileName: tmp}
	// The SSTable is the last (and only) level, so tombstones are physically
	// dropped: nothing below is left to shadow.
	src := NoDeletedSortedKV{SortedKV: kv.levels()}
	if err := nf.CreateFromSorted(src); err != nil {
		nf.Close()
		return err
	}
	nf.Close()

	if err := renameSync(tmp, kv.main.FileName); err != nil {
		return err
	}
	if kv.mainOpen {
		kv.main.Close()
	}
	kv.main = SortedFile{FileName: kv.main.FileName}
	if err := kv.main.Open(); err != nil {
		return err
	}
	kv.mainOpen = true

	kv.mem.Clear()
	return kv.log.Truncate()
}
