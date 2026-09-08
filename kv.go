// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

// KV is the storage engine. Writes are appended to a write-ahead log and mirrored
// in an in-memory map; on Open the log is replayed to rebuild the map, so the
// data survives a crash. Later steps replace the map with sorted on-disk
// structures beneath the same Get/Set/Del surface.
type KV struct {
	log Log
	mem map[string][]byte
}

// Open opens the log, then replays it to rebuild the in-memory map.
func (kv *KV) Open() error {
	if err := kv.log.Open(); err != nil {
		return err
	}
	kv.mem = map[string][]byte{}

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
			delete(kv.mem, string(ent.key))
		} else {
			kv.mem[string(ent.key)] = ent.val
		}
	}
	return nil
}

// Close closes the log.
func (kv *KV) Close() error { return kv.log.Close() }

// Get returns the value for key. ok reports whether the key was present.
func (kv *KV) Get(key []byte) (val []byte, ok bool, err error) {
	val, ok = kv.mem[string(key)]
	return val, ok, nil
}

// UpdateMode selects INSERT / UPDATE / UPSERT semantics for SetEx.
type UpdateMode int

const (
	ModeUpsert UpdateMode = 0 // insert or overwrite (the original Set)
	ModeInsert UpdateMode = 1 // only if the key is absent
	ModeUpdate UpdateMode = 2 // only if the key is present
)

// SetEx writes val under key subject to mode. It reports whether the database
// state actually changed. KV.Set was quietly an upsert all along; SQL cares
// which of the three cases happened, so the storage engine now exposes them.
func (kv *KV) SetEx(key, val []byte, mode UpdateMode) (bool, error) {
	_, existed := kv.mem[string(key)]
	if existed && mode == ModeInsert {
		return false, nil
	}
	if !existed && mode == ModeUpdate {
		return false, nil
	}
	if err := kv.log.Write(&Entry{key: key, val: val}); err != nil {
		return false, err
	}
	kv.mem[string(key)] = val
	return true, nil
}

// Set is an upsert: insert or overwrite, always writing.
func (kv *KV) Set(key []byte, val []byte) (updated bool, err error) {
	return kv.SetEx(key, val, ModeUpsert)
}

// Del removes key: a tombstone record is appended to the log, then the map entry
// is dropped. deleted reports whether a value was actually removed.
func (kv *KV) Del(key []byte) (deleted bool, err error) {
	if err := kv.log.Write(&Entry{key: key, deleted: true}); err != nil {
		return false, err
	}
	_, existed := kv.mem[string(key)]
	delete(kv.mem, string(key))
	return existed, nil
}
