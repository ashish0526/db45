package db

import (
	"bytes"
	"errors"
	"fmt"
)

// evalExpr walks an expression tree and computes its value as a *Cell. This is a
// tree-walking interpreter: recurse the AST, evaluate children, combine at the
// node. Slow (pointer chasing per node) but obviously correct; production engines
// compile the tree to bytecode. What matters here is the semantics.
func evalExpr(schema *Schema, row Row, expr interface{}) (*Cell, error) {
	switch e := expr.(type) {
	case string:
		i := schema.colIndex(e)
		if i < 0 {
			return nil, fmt.Errorf("no such column %q", e)
		}
		c := row[i]
		return &c, nil
	case *Cell:
		return e, nil
	case *ExprBinOp:
		left, err := evalExpr(schema, row, e.left)
		if err != nil {
			return nil, err
		}
		right, err := evalExpr(schema, row, e.right)
		if err != nil {
			return nil, err
		}
		return evalBinOp(e.op, left, right)
	default:
		return nil, fmt.Errorf("cannot evaluate %T", expr)
	}
}

func boolCell(b bool) *Cell {
	if b {
		return &Cell{Type: TypeI64, I64: 1}
	}
	return &Cell{Type: TypeI64, I64: 0}
}

// truthy: a zero int, an empty string are false; everything else true.
func truthy(c *Cell) bool {
	if c.Type == TypeStr {
		return len(c.Str) > 0
	}
	return c.I64 != 0
}

// cmp returns -1/0/1 comparing two same-typed cells.
func cmp(a, b *Cell) (int, error) {
	if a.Type != b.Type {
		return 0, errors.New("cannot compare values of different types")
	}
	if a.Type == TypeStr {
		return bytes.Compare(a.Str, b.Str), nil
	}
	switch {
	case a.I64 < b.I64:
		return -1, nil
	case a.I64 > b.I64:
		return 1, nil
	default:
		return 0, nil
	}
}

func evalBinOp(op ExprOp, left, right *Cell) (*Cell, error) {
	switch op {
	case OP_ADD, OP_SUB, OP_MUL, OP_DIV:
		if left.Type != TypeI64 || right.Type != TypeI64 {
			return nil, errors.New("arithmetic requires integers")
		}
		switch op {
		case OP_ADD:
			return &Cell{Type: TypeI64, I64: left.I64 + right.I64}, nil
		case OP_SUB:
			return &Cell{Type: TypeI64, I64: left.I64 - right.I64}, nil
		case OP_MUL:
			return &Cell{Type: TypeI64, I64: left.I64 * right.I64}, nil
		default:
			if right.I64 == 0 {
				return nil, errors.New("division by zero")
			}
			return &Cell{Type: TypeI64, I64: left.I64 / right.I64}, nil
		}

	case OP_LT, OP_LE, OP_GT, OP_GE:
		r, err := cmp(left, right)
		if err != nil {
			return nil, err
		}
		switch op {
		case OP_LT:
			return boolCell(r < 0), nil
		case OP_LE:
			return boolCell(r <= 0), nil
		case OP_GT:
			return boolCell(r > 0), nil
		default:
			return boolCell(r >= 0), nil
		}
	}
	return nil, fmt.Errorf("unknown operator %d", op)
}
