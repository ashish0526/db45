package db

import "bytes"

// MergedSortedKV presents k sorted levels as one sorted stream. Level 0 is the
// newest / highest priority; on a tie it wins and the lower copies are skipped,
// so the merged stream has no duplicate keys. Merge sort is the heart of an LSM
// range read: the next key in order can live in any level, so you keep one
// cursor per level and repeatedly emit the extreme current key.
type MergedSortedKV []SortedKV

// Size is the sum of level sizes — an over-estimate once duplicates and
// tombstones are removed.
func (m MergedSortedKV) Size() int {
	n := 0
	for _, s := range m {
		n += s.Size()
	}
	return n
}

// Iter returns a merged cursor positioned at the start.
func (m MergedSortedKV) Iter() (SortedKVIter, error) {
	levels := make([]SortedKVIter, len(m))
	for i, sub := range m {
		it, err := sub.Iter()
		if err != nil {
			return nil, err
		}
		levels[i] = it
	}
	mit := &MergedSortedKVIter{levels: levels}
	mit.which = mit.extreme()
	mit.captureFrontier()
	return mit, nil
}

// MergedSortedKVIter merges level cursors. which is the level currently holding
// the emitted key (-1 when exhausted); frontier is the key at the current
// position (nil when exhausted); desc is the current direction, which can flip
// mid-iteration.
type MergedSortedKVIter struct {
	levels   []SortedKVIter
	which    int
	frontier []byte
	desc     bool
}

// captureFrontier records the current key so a later direction flip can
// reposition every level relative to it.
func (it *MergedSortedKVIter) captureFrontier() {
	if it.which < 0 {
		it.frontier = nil
		return
	}
	it.frontier = append([]byte(nil), it.Key()...)
}

func (it *MergedSortedKVIter) Valid() bool { return it.which >= 0 }
func (it *MergedSortedKVIter) Key() []byte { return it.levels[it.which].Key() }
func (it *MergedSortedKVIter) Val() []byte { return it.levels[it.which].Val() }

// extreme returns the index of the level holding the smallest (ascending) or
// largest (descending) current key, ties won by the lowest index, or -1.
func (it *MergedSortedKVIter) extreme() int {
	best := -1
	for i, lv := range it.levels {
		if !lv.Valid() {
			continue
		}
		if best == -1 {
			best = i
			continue
		}
		c := bytes.Compare(lv.Key(), it.levels[best].Key())
		if (!it.desc && c < 0) || (it.desc && c > 0) {
			best = i
		}
	}
	return best
}

func (it *MergedSortedKVIter) Next() error { return it.step(false) }
func (it *MergedSortedKVIter) Prev() error { return it.step(true) }

// step advances the merge one position. desc says the requested direction; a
// change from it.desc triggers a reorientation of every level cursor relative to
// the last emitted key.
func (it *MergedSortedKVIter) step(desc bool) error {
	if desc != it.desc {
		it.desc = desc
		if it.frontier == nil {
			it.which = -1 // stepped off the end we started from
			return nil
		}
		for _, lv := range it.levels {
			if err := reorient(lv, it.frontier, desc); err != nil {
				return err
			}
		}
		it.which = it.extreme()
		it.captureFrontier()
		return nil
	}

	if it.which < 0 {
		return nil
	}
	key := append([]byte(nil), it.Key()...)
	// Advance every level still sitting on the current key: this steps past it
	// and skips the shadowed lower copies in one move.
	for _, lv := range it.levels {
		if lv.Valid() && bytes.Equal(lv.Key(), key) {
			if err := advance(lv, desc); err != nil {
				return err
			}
		}
	}
	it.which = it.extreme()
	it.captureFrontier()
	return nil
}

func advance(lv SortedKVIter, desc bool) error {
	if desc {
		return lv.Prev()
	}
	return lv.Next()
}

// reorient moves lv to the first key strictly past `key` in direction desc,
// first stepping back into range if lv had run off the opposite end.
func reorient(lv SortedKVIter, key []byte, desc bool) error {
	if desc {
		if !lv.Valid() {
			if err := lv.Prev(); err != nil { // from one-past-the-top back to the last key
				return err
			}
		}
		for lv.Valid() && bytes.Compare(lv.Key(), key) >= 0 {
			if err := lv.Prev(); err != nil {
				return err
			}
		}
		return nil
	}
	if !lv.Valid() {
		if err := lv.Next(); err != nil { // from one-before-the-start to the first key
			return err
		}
	}
	for lv.Valid() && bytes.Compare(lv.Key(), key) <= 0 {
		if err := lv.Next(); err != nil {
			return err
		}
	}
	return nil
}
