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

// Set writes val under key: first durably to the log, then to the map. updated
// reports whether the key already existed.
func (kv *KV) Set(key []byte, val []byte) (updated bool, err error) {
	if err := kv.log.Write(&Entry{key: key, val: val}); err != nil {
		return false, err
	}
	_, existed := kv.mem[string(key)]
	kv.mem[string(key)] = val
	return existed, nil
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
