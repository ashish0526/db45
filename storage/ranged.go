package storage

import "bytes"

// RangedKVIter is a SortedKVIter restricted to a closed byte-key interval and a
// direction. Ascending: start <= key <= stop, moving forward. Descending:
// start >= key >= stop, moving backward (ORDER BY ... DESC). The schema-aware
// side of range queries lives in the sql package.
type RangedKVIter struct {
	iter SortedKVIter
	stop []byte
	desc bool
}

func (it *RangedKVIter) Key() []byte   { return it.iter.Key() }
func (it *RangedKVIter) Val() []byte   { return it.iter.Val() }
func (it *RangedKVIter) Deleted() bool { return it.iter.Deleted() }

// Prev steps against the scan direction (used when a RowIterator reverses).
func (it *RangedKVIter) Prev() error {
	if it.desc {
		return it.iter.Next()
	}
	return it.iter.Prev()
}

func (it *RangedKVIter) Valid() bool {
	if !it.iter.Valid() {
		return false
	}
	r := bytes.Compare(it.iter.Key(), it.stop)
	if it.desc && r < 0 {
		return false
	}
	if !it.desc && r > 0 {
		return false
	}
	return true
}

func (it *RangedKVIter) Next() error {
	if !it.Valid() {
		return nil
	}
	if it.desc {
		return it.iter.Prev()
	}
	return it.iter.Next()
}

// Range returns an iterator over the closed byte-key interval [start, stop]
// (ascending) or [stop, start] walked backward (descending). Seek is always >=,
// so a descending scan is fixed up to the last key <= start.
func (kv *KV) Range(start, stop []byte, desc bool) (*RangedKVIter, error) {
	kvit, err := kv.Seek(start)
	if err != nil {
		return nil, err
	}
	if desc {
		if !kvit.Valid() || bytes.Compare(kvit.Key(), start) > 0 {
			_ = kvit.Prev()
		}
	}
	return &RangedKVIter{iter: kvit, stop: stop, desc: desc}, nil
}
