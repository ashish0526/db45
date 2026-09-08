package db

// filterDeleted wraps a cursor so tombstones never surface to a reader. The
// merge still emits them (so they can shadow lower levels); this is the last
// stage that removes them from query output.
func filterDeleted(iter SortedKVIter) (SortedKVIter, error) {
	for iter.Valid() && iter.Deleted() {
		if err := iter.Next(); err != nil {
			return nil, err
		}
	}
	return &NoDeletedIter{SortedKVIter: iter}, nil
}

// NoDeletedIter embeds a SortedKVIter and overrides only Next/Prev to skip
// further tombstones; Key/Val/Valid/Deleted pass through (Go embedding as
// decoration).
type NoDeletedIter struct {
	SortedKVIter
}

func (it *NoDeletedIter) Next() error {
	if err := it.SortedKVIter.Next(); err != nil {
		return err
	}
	for it.Valid() && it.Deleted() {
		if err := it.SortedKVIter.Next(); err != nil {
			return err
		}
	}
	return nil
}

func (it *NoDeletedIter) Prev() error {
	if err := it.SortedKVIter.Prev(); err != nil {
		return err
	}
	for it.Valid() && it.Deleted() {
		if err := it.SortedKVIter.Prev(); err != nil {
			return err
		}
	}
	return nil
}

// NoDeletedSortedKV wraps a SortedKV so its cursors physically omit tombstones —
// used when merging into the final level, where a tombstone has nothing left to
// shadow.
type NoDeletedSortedKV struct {
	SortedKV
}

func (kv NoDeletedSortedKV) Iter() (SortedKVIter, error) {
	it, err := kv.SortedKV.Iter()
	if err != nil {
		return nil, err
	}
	return filterDeleted(it)
}

func (kv NoDeletedSortedKV) Seek(key []byte) (SortedKVIter, error) {
	it, err := kv.SortedKV.Seek(key)
	if err != nil {
		return nil, err
	}
	return filterDeleted(it)
}
