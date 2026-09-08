// Package db is a database built in 45 steps: a relational engine layered over a
// sorted key/value store. This file is the storage-engine API.
package db

// KV is the storage engine. Step 0101 backs it with a plain in-memory map; later
// steps add a write-ahead log, sorted on-disk structures and an LSM-Tree beneath
// the same Get/Set/Del surface.
type KV struct {
	mem map[string][]byte
}

// Open readies the store for use.
func (kv *KV) Open() error {
	kv.mem = map[string][]byte{}
	return nil
}

// Close releases the store. Nothing to do for an in-memory map.
func (kv *KV) Close() error { return nil }

// Get returns the value for key. ok reports whether the key was present.
func (kv *KV) Get(key []byte) (val []byte, ok bool, err error) {
	val, ok = kv.mem[string(key)]
	return val, ok, nil
}

// Set writes val under key. updated reports whether the database state changed
// (here: whether the key already existed). This "did anything happen?" bit
// propagates all the way up to SQL's "rows affected".
func (kv *KV) Set(key []byte, val []byte) (updated bool, err error) {
	_, existed := kv.mem[string(key)]
	kv.mem[string(key)] = val
	return existed, nil
}

// Del removes key. deleted reports whether a value was actually removed.
func (kv *KV) Del(key []byte) (deleted bool, err error) {
	_, existed := kv.mem[string(key)]
	delete(kv.mem, string(key))
	return existed, nil
}
