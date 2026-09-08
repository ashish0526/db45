// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

// KV is the storage engine. Recent writes live in an in-memory SortedArray (the
// MemTable); the write-ahead log makes them durable and is replayed on Open.
// Chapters 6-7 add immutable on-disk SSTables beneath the same Get/Set/Del
// surface.
type KV struct {
	log Log
	mem SortedArray
}

// Open opens the log, then replays it into the MemTable.
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
	return nil
}

// Close closes the log.
func (kv *KV) Close() error { return kv.log.Close() }

// Get returns the value for key. ok reports whether the key was present.
func (kv *KV) Get(key []byte) (val []byte, ok bool, err error) {
	return kv.mem.Get(key)
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
	_, existed := kv.mem.search(key)
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

// Del removes key. deleted reports whether a value was actually removed.
func (kv *KV) Del(key []byte) (deleted bool, err error) {
	if _, existed := kv.mem.search(key); !existed {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, deleted: true}); err != nil {
		return false, err
	}
	kv.mem.Del(key)
	return true, nil
}

// Seek returns a cursor over the store, positioned at the first key >= key.
func (kv *KV) Seek(key []byte) (SortedKVIter, error) {
	return kv.mem.Seek(key)
}
