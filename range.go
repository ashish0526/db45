package db

import "bytes"

// ExprOp identifies an operator. Chapter 5 adds arithmetic, equality and boolean
// operators; for now only the four range comparisons, numbered as in the lesson.
type ExprOp uint8

const (
	OP_ADD ExprOp = 1
	OP_SUB ExprOp = 2
	OP_MUL ExprOp = 3
	OP_DIV ExprOp = 4

	OP_LE ExprOp = 12
	OP_GE ExprOp = 13
	OP_LT ExprOp = 14
	OP_GT ExprOp = 15
)

// RangedKVIter is a KVIterator restricted to a closed byte-key interval and a
// direction. Ascending: start <= key <= stop, moving forward. Descending:
// start >= key >= stop, moving backward (ORDER BY ... DESC).
type RangedKVIter struct {
	iter *KVIterator
	stop []byte
	desc bool
}

func (it *RangedKVIter) Key() []byte { return it.iter.Key() }
func (it *RangedKVIter) Val() []byte { return it.iter.Val() }

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
		// Seek landed on the first key >= start; step back unless it is exactly
		// start, so the cursor sits on the last key <= start.
		if !kvit.Valid() || bytes.Compare(kvit.Key(), start) > 0 {
			_ = kvit.Prev()
		}
	}
	return &RangedKVIter{iter: kvit, stop: stop, desc: desc}, nil
}

// RangeReq is a DB-level range request over full keys or prefixes of a primary
// key. StartCmp decides the direction.
type RangeReq struct {
	StartCmp, StopCmp ExprOp
	Start, Stop       []Cell
}

// isDescending reports whether a start comparison scans downward.
func isDescending(op ExprOp) bool { return op == OP_LE || op == OP_LT }

// suffixPositive turns a comparison operator into its ±infinity padding choice:
//
//	>= x  ->  >= (x, -inf)      <= x  ->  <= (x, +inf)
//	>  x  ->  >= (x, +inf)      <  x  ->  <= (x, -inf)
func suffixPositive(op ExprOp) bool { return op == OP_GT || op == OP_LE }

// Range translates a RangeReq into byte-key bounds and returns a RowIterator that
// yields the matching rows in order.
func (db *DB) Range(schema *Schema, req *RangeReq) (*RowIterator, error) {
	desc := isDescending(req.StartCmp)
	start := EncodeKeyPrefix(schema, req.Start, suffixPositive(req.StartCmp))
	stop := EncodeKeyPrefix(schema, req.Stop, suffixPositive(req.StopCmp))

	rit, err := db.KV.Range(start, stop, desc)
	if err != nil {
		return nil, err
	}
	it := &RowIterator{schema: schema, iter: rit}
	if err := it.decode(); err != nil {
		return nil, err
	}
	return it, nil
}
