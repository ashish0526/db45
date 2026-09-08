package sql

import (
	"errors"

	"github.com/ashish0526/db45/table"
)

// RangeReq is a range request over full keys or prefixes of a primary key.
// StartCmp decides the direction.
type RangeReq struct {
	StartCmp, StopCmp ExprOp
	Start, Stop       []table.Cell
}

// isDescending reports whether a start comparison scans downward.
func isDescending(op ExprOp) bool { return op == OP_LE || op == OP_LT }

// suffixPositive turns a comparison operator into its ±infinity padding choice:
//
//	>= x  ->  >= (x, -inf)      <= x  ->  <= (x, +inf)
//	>  x  ->  >= (x, +inf)      <  x  ->  <= (x, -inf)
func suffixPositive(op ExprOp) bool { return op == OP_GT || op == OP_LE }

// Range translates a RangeReq into byte-key bounds and returns a RowIterator over
// the matching rows. This is the crux of every real range scan: a multi-column
// key comparison reduced to one byte-string comparison via an order-preserving
// concatenated encoding plus sentinel bytes for the open end.
func Range(db *table.DB, schema *table.Schema, req *RangeReq) (*table.RowIterator, error) {
	desc := isDescending(req.StartCmp)
	start := table.EncodeKeyPrefix(schema, req.Start, suffixPositive(req.StartCmp))
	stop := table.EncodeKeyPrefix(schema, req.Stop, suffixPositive(req.StopCmp))

	rit, err := db.KV.Range(start, stop, desc)
	if err != nil {
		return nil, err
	}
	return table.NewRowIterator(schema, rit)
}

// ExprTuple is a parenthesised list: `(a, b)` or `(1, 2)`. A single-element
// parenthesised expression is just grouping and never becomes a tuple.
type ExprTuple struct {
	kids []interface{}
}

// parseTuple parses `( expr (, expr)* )`. With one element it returns that
// element (grouping); with more it returns an *ExprTuple.
func (p *Parser) parseTuple() (interface{}, error) {
	if !p.tryPunctuation("(") {
		return nil, errors.New("expect (")
	}
	var kids []interface{}
	err := p.commaList(func() error {
		e, err := p.parseExpr()
		if err != nil {
			return err
		}
		kids = append(kids, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !p.tryPunctuation(")") {
		return nil, errors.New("expect )")
	}
	if len(kids) == 1 {
		return kids[0], nil
	}
	return &ExprTuple{kids: kids}, nil
}

// isPKeyPrefix reports whether cols names the first len(cols) primary-key
// columns, in order.
func isPKeyPrefix(schema *table.Schema, cols []string) bool {
	if len(cols) == 0 || len(cols) > len(schema.PKey) {
		return false
	}
	for i, name := range cols {
		if schema.Cols[schema.PKey[i]].Name != name {
			return false
		}
	}
	return true
}

// destructureCmp splits the two sides of a comparison into column names and
// constant cells: `col OP const` or `(a,b) OP (x,y)`.
func destructureCmp(left, right interface{}) (cols []string, vals []*table.Cell, ok bool) {
	switch l := left.(type) {
	case string:
		r, okr := right.(*table.Cell)
		if !okr {
			return nil, nil, false
		}
		return []string{l}, []*table.Cell{r}, true
	case *ExprTuple:
		r, okr := right.(*ExprTuple)
		if !okr || len(r.kids) != len(l.kids) {
			return nil, nil, false
		}
		for i := range l.kids {
			name, okn := l.kids[i].(string)
			val, okv := r.kids[i].(*table.Cell)
			if !okn || !okv {
				return nil, nil, false
			}
			cols = append(cols, name)
			vals = append(vals, val)
		}
		return cols, vals, true
	default:
		return nil, nil, false
	}
}

// typedPrefix returns the constant cells retyped to their primary-key columns.
func typedPrefix(schema *table.Schema, vals []*table.Cell) []table.Cell {
	out := make([]table.Cell, len(vals))
	for i := range vals {
		out[i] = *vals[i]
		out[i].Type = schema.Cols[schema.PKey[i]].Type
	}
	return out
}

// cmpToBound turns one comparison into a start or stop bound.
func cmpToBound(schema *table.Schema, cond interface{}) (op ExprOp, cells []table.Cell, isStart, ok bool) {
	b, isBin := cond.(*ExprBinOp)
	if !isBin {
		return 0, nil, false, false
	}
	switch b.op {
	case OP_GT, OP_GE, OP_LT, OP_LE:
	default:
		return 0, nil, false, false
	}
	cols, vals, okd := destructureCmp(b.left, b.right)
	if !okd || !isPKeyPrefix(schema, cols) {
		return 0, nil, false, false
	}
	return b.op, typedPrefix(schema, vals), b.op == OP_GT || b.op == OP_GE, true
}

// extractPKey turns an all-equalities match into a full primary key (all PK
// columns, in order), if it is exactly that.
func extractPKey(schema *table.Schema, keys []NamedCell) ([]table.Cell, bool) {
	if len(keys) != len(schema.PKey) {
		return nil, false
	}
	out := make([]table.Cell, len(schema.PKey))
	for i, pk := range schema.PKey {
		name := schema.Cols[pk].Name
		found := false
		for _, nc := range keys {
			if nc.column == name {
				out[i] = nc.value
				out[i].Type = schema.Cols[pk].Type
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return out, true
}

// matchRange recognises `a > 1`, `(a,b) >= (1,2)`, or a two-sided
// `a > 1 AND a < 9` as an ascending range over a primary-key prefix.
func matchRange(schema *table.Schema, cond interface{}) (*RangeReq, bool) {
	req := &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE} // -inf .. +inf
	apply := func(c interface{}) bool {
		op, cells, isStart, ok := cmpToBound(schema, c)
		if !ok {
			return false
		}
		if isStart {
			req.StartCmp, req.Start = op, cells
		} else {
			req.StopCmp, req.Stop = op, cells
		}
		return true
	}
	if b, ok := cond.(*ExprBinOp); ok && b.op == OP_AND {
		if apply(b.left) && apply(b.right) {
			return req, true
		}
		return nil, false
	}
	if _, ok := cond.(*ExprBinOp); ok {
		if apply(cond) {
			return req, true
		}
	}
	return nil, false
}

// makeRange picks an access path for a WHERE expression: a full scan (no WHERE),
// a point lookup (all PK columns equated), or a range over a PK prefix.
// OR would need a union of ranges — the lesson stops here on purpose.
func makeRange(schema *table.Schema, cond interface{}) (*RangeReq, error) {
	if cond == nil {
		return &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE}, nil // full scan
	}
	if keys, ok := matchAllEq(cond, nil); ok {
		if pkey, ok := extractPKey(schema, keys); ok {
			// a point lookup is a zero-width range
			return &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE, Start: pkey, Stop: pkey}, nil
		}
	}
	if req, ok := matchRange(schema, cond); ok {
		return req, nil
	}
	return nil, errors.New("unimplemented WHERE")
}

// execCond returns a RowIterator over the rows a WHERE expression selects.
func execCond(db *table.DB, schema *table.Schema, cond interface{}) (*table.RowIterator, error) {
	req, err := makeRange(schema, cond)
	if err != nil {
		return nil, err
	}
	return Range(db, schema, req)
}
