package db

import "errors"

// This is query analysis / planning in miniature: the parser accepts any boolean
// WHERE expression, and the executor then pattern-matches the tree for a shape it
// can serve efficiently. Step 0506 recognises one shape — "every column equated
// to a constant" => a primary-key point lookup. Step 0507 adds ranges. Anything
// unrecognised is rejected (a real optimizer would fall back to a full scan +
// filter).

// matchAllEq recognises `col = const AND col = const AND ...` and returns the
// (column, value) pairs.
func matchAllEq(cond interface{}, out []NamedCell) ([]NamedCell, bool) {
	binop, ok := cond.(*ExprBinOp)
	if !ok {
		return nil, false
	}
	switch binop.op {
	case OP_AND:
		var okL, okR bool
		if out, okL = matchAllEq(binop.left, out); !okL {
			return nil, false
		}
		if out, okR = matchAllEq(binop.right, out); !okR {
			return nil, false
		}
		return out, true
	case OP_EQ:
		col, okC := binop.left.(string)
		val, okV := binop.right.(*Cell)
		if !okC || !okV {
			return nil, false
		}
		return append(out, NamedCell{column: col, value: *val}), true
	default:
		return nil, false
	}
}

// matchPKey recovers a primary-key Row from a general WHERE expression, or errors
// if the expression is not a recognised point lookup.
func matchPKey(schema *Schema, cond interface{}) (Row, error) {
	if cond == nil {
		return nil, errors.New("WHERE clause required")
	}
	keys, ok := matchAllEq(cond, nil)
	if !ok {
		return nil, errors.New("unimplemented WHERE (expected col = const AND ...)")
	}
	return makePKey(schema, keys)
}
