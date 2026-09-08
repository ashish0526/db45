package sql

import "github.com/ashish0526/db45/table"

// This is query analysis / planning in miniature: the parser accepts any boolean
// WHERE expression, and the executor (see makerange.go) then pattern-matches the
// tree for a shape it can serve efficiently — an all-equalities point lookup, a
// PK-prefix range, or a full scan. Anything unrecognised is rejected (a real
// optimizer would fall back to a full scan + residual filter).

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
		val, okV := binop.right.(*table.Cell)
		if !okC || !okV {
			return nil, false
		}
		return append(out, NamedCell{column: col, value: *val}), true
	default:
		return nil, false
	}
}
