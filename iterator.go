package db

import (
	"bytes"
	"slices"
)

// KVIterator is a cursor over the sorted KV store. pos is allowed to sit one
// past either end (-1 or len): Valid() reports false there, but a single
// Next()/Prev() steps back into range, so boundary handling and descending
// iteration fall out without special cases. Next/Prev return error because on
// disk (Chapter 6) each step can do I/O.
type KVIterator struct {
	keys [][]byte
	vals [][]byte
	pos  int
}

// Seek returns a cursor positioned at the first key >= key.
func (kv *KV) Seek(key []byte) (*KVIterator, error) {
	pos, _ := slices.BinarySearchFunc(kv.keys, key, bytes.Compare)
	return &KVIterator{keys: kv.keys, vals: kv.vals, pos: pos}, nil
}

func (it *KVIterator) Valid() bool { return 0 <= it.pos && it.pos < len(it.keys) }
func (it *KVIterator) Key() []byte { return it.keys[it.pos] }
func (it *KVIterator) Val() []byte { return it.vals[it.pos] }

func (it *KVIterator) Next() error {
	if it.pos < len(it.keys) {
		it.pos++
	}
	return nil
}

func (it *KVIterator) Prev() error {
	if it.pos >= 0 {
		it.pos--
	}
	return nil
}
