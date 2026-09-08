package db

import (
	"bytes"
	"slices"
)

// SortedArray is the in-memory sorted KV structure (the MemTable), extracted
// into its own type so it can be swapped for something better later and so it
// satisfies the SortedKV interface the SSTable machinery consumes. Rule for the
// rest of the project: touch a SortedArray through its methods, never its fields.
type SortedArray struct {
	keys [][]byte
	vals [][]byte
}

func (a *SortedArray) Size() int        { return len(a.keys) }
func (a *SortedArray) Key(i int) []byte { return a.keys[i] }
func (a *SortedArray) Val(i int) []byte { return a.vals[i] }

// Clear empties the array (keeping capacity).
func (a *SortedArray) Clear() {
	a.keys = a.keys[:0]
	a.vals = a.vals[:0]
}

// Push appends a pair to the end — the caller guarantees sorted order.
func (a *SortedArray) Push(key, val []byte) {
	a.keys = append(a.keys, key)
	a.vals = append(a.vals, val)
}

// Pop drops the last pair.
func (a *SortedArray) Pop() {
	a.keys = a.keys[:len(a.keys)-1]
	a.vals = a.vals[:len(a.vals)-1]
}

func (a *SortedArray) search(key []byte) (int, bool) {
	return slices.BinarySearchFunc(a.keys, key, bytes.Compare)
}

func (a *SortedArray) Get(key []byte) (val []byte, ok bool, err error) {
	if i, found := a.search(key); found {
		return a.vals[i], true, nil
	}
	return nil, false, nil
}

// Set inserts or overwrites, keeping keys sorted. updated reports an overwrite.
func (a *SortedArray) Set(key, val []byte) (updated bool, err error) {
	i, found := a.search(key)
	if found {
		a.vals[i] = val
		return true, nil
	}
	a.keys = slices.Insert(a.keys, i, key)
	a.vals = slices.Insert(a.vals, i, val)
	return false, nil
}

func (a *SortedArray) Del(key []byte) (deleted bool, err error) {
	i, found := a.search(key)
	if !found {
		return false, nil
	}
	a.keys = slices.Delete(a.keys, i, i+1)
	a.vals = slices.Delete(a.vals, i, i+1)
	return true, nil
}

// Iter returns a cursor at the start.
func (a *SortedArray) Iter() (SortedKVIter, error) {
	return &SortedArrayIter{keys: a.keys, vals: a.vals, pos: 0}, nil
}

// Seek returns a cursor at the first key >= key.
func (a *SortedArray) Seek(key []byte) (SortedKVIter, error) {
	pos, _ := a.search(key)
	return &SortedArrayIter{keys: a.keys, vals: a.vals, pos: pos}, nil
}

// SortedArrayIter is a cursor over a SortedArray (formerly KVIterator). pos may
// sit one past either end: Valid() reports false there, but a single
// Next()/Prev() steps back into range, so descending iteration and boundary
// handling need no special cases. Next/Prev return error because an on-disk
// cursor's step can do I/O.
type SortedArrayIter struct {
	keys [][]byte
	vals [][]byte
	pos  int
}

func (it *SortedArrayIter) Valid() bool { return 0 <= it.pos && it.pos < len(it.keys) }
func (it *SortedArrayIter) Key() []byte { return it.keys[it.pos] }
func (it *SortedArrayIter) Val() []byte { return it.vals[it.pos] }

func (it *SortedArrayIter) Next() error {
	if it.pos < len(it.keys) {
		it.pos++
	}
	return nil
}

func (it *SortedArrayIter) Prev() error {
	if it.pos >= 0 {
		it.pos--
	}
	return nil
}
