package db

import (
	"bytes"
	"slices"
)

// SortedArray is the in-memory sorted KV structure (the MemTable). It now also
// stores a per-key tombstone flag: you cannot delete a key from a lower level,
// so Del records a `deleted = true` entry that shadows lower levels during a
// merge and is filtered out of query output. Rule for the rest of the project:
// touch a SortedArray through its methods, never its fields.
type SortedArray struct {
	keys    [][]byte
	vals    [][]byte
	deleted []bool
}

// Size is the exact entry count (including tombstones).
func (a *SortedArray) Size() int { return len(a.keys) }

// EstimatedSize satisfies SortedKV. For an array it is exact.
func (a *SortedArray) EstimatedSize() int { return len(a.keys) }

func (a *SortedArray) Key(i int) []byte   { return a.keys[i] }
func (a *SortedArray) Val(i int) []byte   { return a.vals[i] }
func (a *SortedArray) Deleted(i int) bool { return a.deleted[i] }

// Clear empties the array (keeping capacity).
func (a *SortedArray) Clear() {
	a.keys = a.keys[:0]
	a.vals = a.vals[:0]
	a.deleted = a.deleted[:0]
}

// Push appends a triple to the end — the caller guarantees sorted order.
func (a *SortedArray) Push(key, val []byte, deleted bool) {
	a.keys = append(a.keys, key)
	a.vals = append(a.vals, val)
	a.deleted = append(a.deleted, deleted)
}

// Pop drops the last triple.
func (a *SortedArray) Pop() {
	a.keys = a.keys[:len(a.keys)-1]
	a.vals = a.vals[:len(a.vals)-1]
	a.deleted = a.deleted[:len(a.deleted)-1]
}

func (a *SortedArray) search(key []byte) (int, bool) {
	return slices.BinarySearchFunc(a.keys, key, bytes.Compare)
}

// entryState reports whether key is present, tombstoned, or absent in this array.
type entryState int

const (
	entryAbsent entryState = iota
	entryPresent
	entryTombstone
)

func (a *SortedArray) state(key []byte) (entryState, []byte) {
	i, found := a.search(key)
	if !found {
		return entryAbsent, nil
	}
	if a.deleted[i] {
		return entryTombstone, nil
	}
	return entryPresent, a.vals[i]
}

// Get is a MemTable-only lookup: ok is false for both absent and tombstoned keys.
func (a *SortedArray) Get(key []byte) (val []byte, ok bool, err error) {
	st, v := a.state(key)
	return v, st == entryPresent, nil
}

// Set inserts or overwrites a live value, clearing any tombstone.
func (a *SortedArray) Set(key, val []byte) (updated bool, err error) {
	i, found := a.search(key)
	if found {
		wasLive := !a.deleted[i]
		a.vals[i] = val
		a.deleted[i] = false
		return wasLive, nil
	}
	a.keys = slices.Insert(a.keys, i, key)
	a.vals = slices.Insert(a.vals, i, val)
	a.deleted = slices.Insert(a.deleted, i, false)
	return false, nil
}

// Del records a tombstone for key (marking, not removing — a lower level may
// still hold it).
func (a *SortedArray) Del(key []byte) {
	i, found := a.search(key)
	if found {
		a.vals[i] = nil
		a.deleted[i] = true
		return
	}
	a.keys = slices.Insert(a.keys, i, key)
	a.vals = slices.Insert(a.vals, i, nil)
	a.deleted = slices.Insert(a.deleted, i, true)
}

// Iter returns a cursor at the start.
func (a *SortedArray) Iter() (SortedKVIter, error) {
	return &SortedArrayIter{a: a, pos: 0}, nil
}

// Seek returns a cursor at the first key >= key.
func (a *SortedArray) Seek(key []byte) (SortedKVIter, error) {
	pos, _ := a.search(key)
	return &SortedArrayIter{a: a, pos: pos}, nil
}

// SortedArrayIter is a cursor over a SortedArray. pos may sit one past either
// end: Valid() reports false there, but a single Next()/Prev() steps back into
// range. Next/Prev return error because an on-disk cursor's step can do I/O.
type SortedArrayIter struct {
	a   *SortedArray
	pos int
}

func (it *SortedArrayIter) Valid() bool   { return 0 <= it.pos && it.pos < len(it.a.keys) }
func (it *SortedArrayIter) Key() []byte   { return it.a.keys[it.pos] }
func (it *SortedArrayIter) Val() []byte   { return it.a.vals[it.pos] }
func (it *SortedArrayIter) Deleted() bool { return it.a.deleted[it.pos] }

func (it *SortedArrayIter) Next() error {
	if it.pos < len(it.a.keys) {
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
