// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

import (
	"bytes"
	"slices"
)

// KV is the storage engine. Recent writes live in a pair of parallel slices kept
// in sorted key order (replacing the Chapter 1-3 map); the write-ahead log still
// makes them durable and is replayed on Open. Keeping keys sorted is what makes
// range queries — and everything built on them — possible. Insert/delete shift
// the tail, so writes are O(N): deliberately bad, to motivate the on-disk
// B+Tree / LSM-Tree of Chapters 6-7.
type KV struct {
	log  Log
	keys [][]byte
	vals [][]byte
}

// Open opens the log, then replays it into the sorted arrays.
func (kv *KV) Open() error {
	if err := kv.log.Open(); err != nil {
		return err
	}
	kv.keys = kv.keys[:0]
	kv.vals = kv.vals[:0]

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
			kv.applyDel(ent.key)
		} else {
			kv.applySet(ent.key, ent.val)
		}
	}
	return nil
}

// Close closes the log.
func (kv *KV) Close() error { return kv.log.Close() }

// search returns the insertion point for key and whether it is already present.
func (kv *KV) search(key []byte) (int, bool) {
	return slices.BinarySearchFunc(kv.keys, key, bytes.Compare)
}

// applySet / applyDel mutate the in-memory arrays only (used by log replay and,
// after the log write, by SetEx/Del).
func (kv *KV) applySet(key, val []byte) bool {
	idx, exist := kv.search(key)
	if exist {
		kv.vals[idx] = val
		return true
	}
	kv.keys = slices.Insert(kv.keys, idx, key)
	kv.vals = slices.Insert(kv.vals, idx, val)
	return false
}

func (kv *KV) applyDel(key []byte) bool {
	idx, exist := kv.search(key)
	if !exist {
		return false
	}
	kv.keys = slices.Delete(kv.keys, idx, idx+1)
	kv.vals = slices.Delete(kv.vals, idx, idx+1)
	return true
}

// Get returns the value for key. ok reports whether the key was present.
func (kv *KV) Get(key []byte) (val []byte, ok bool, err error) {
	if idx, exist := kv.search(key); exist {
		return kv.vals[idx], true, nil
	}
	return nil, false, nil
}

// UpdateMode selects INSERT / UPDATE / UPSERT semantics for SetEx.
type UpdateMode int

const (
	ModeUpsert UpdateMode = 0 // insert or overwrite (the original Set)
	ModeInsert UpdateMode = 1 // only if the key is absent
	ModeUpdate UpdateMode = 2 // only if the key is present
)

// SetEx writes val under key subject to mode. It reports whether a write
// happened.
func (kv *KV) SetEx(key, val []byte, mode UpdateMode) (bool, error) {
	_, existed := kv.search(key)
	if existed && mode == ModeInsert {
		return false, nil
	}
	if !existed && mode == ModeUpdate {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, val: val}); err != nil {
		return false, err
	}
	kv.applySet(key, val)
	return true, nil
}

// Set is an upsert: insert or overwrite, always writing.
func (kv *KV) Set(key []byte, val []byte) (updated bool, err error) {
	return kv.SetEx(key, val, ModeUpsert)
}

// Del removes key. deleted reports whether a value was actually removed.
func (kv *KV) Del(key []byte) (deleted bool, err error) {
	if _, existed := kv.search(key); !existed {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, deleted: true}); err != nil {
		return false, err
	}
	kv.applyDel(key)
	return true, nil
}
