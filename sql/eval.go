package sql

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ashish0526/db45/table"
)

// evalExpr walks an expression tree and computes its value as a *table.Cell. This is a
// tree-walking interpreter: recurse the AST, evaluate children, combine at the
// node. Slow (pointer chasing per node) but obviously correct; production engines
// compile the tree to bytecode. What matters here is the semantics.
func evalExpr(schema *table.Schema, row table.Row, expr interface{}) (*table.Cell, error) {
	switch e := expr.(type) {
	case string:
		i := schema.ColIndex(e)
		if i < 0 {
			return nil, fmt.Errorf("no such column %q", e)
		}
		c := row[i]
		return &c, nil
	case *table.Cell:
		return e, nil
	case *ExprBinOp:
		left, err := evalExpr(schema, row, e.left)
		if err != nil {
			return nil, err
		}
		// AND / OR short-circuit on the left operand.
		if e.op == OP_AND && !truthy(left) {
			return boolCell(false), nil
		}
		if e.op == OP_OR && truthy(left) {
			return boolCell(true), nil
		}
		right, err := evalExpr(schema, row, e.right)
		if err != nil {
			return nil, err
		}
		return evalBinOp(e.op, left, right)
	case *ExprUnOp:
		kid, err := evalExpr(schema, row, e.kid)
		if err != nil {
			return nil, err
		}
		switch e.op {
		case OP_NOT:
			return boolCell(!truthy(kid)), nil
		case OP_NEG:
			if kid.Type != table.TypeI64 {
				return nil, errors.New("unary minus requires an integer")
			}
			return &table.Cell{Type: table.TypeI64, I64: -kid.I64}, nil
		default:
			return nil, fmt.Errorf("unknown unary operator %d", e.op)
		}
	default:
		return nil, fmt.Errorf("cannot evaluate %T", expr)
	}
}

func boolCell(b bool) *table.Cell {
	if b {
		return &table.Cell{Type: table.TypeI64, I64: 1}
	}
	return &table.Cell{Type: table.TypeI64, I64: 0}
}

// truthy: a zero int, an empty string are false; everything else true.
func truthy(c *table.Cell) bool {
	if c.Type == table.TypeStr {
		return len(c.Str) > 0
	}
	return c.I64 != 0
}

// cmp returns -1/0/1 comparing two same-typed cells.
func cmp(a, b *table.Cell) (int, error) {
	if a.Type != b.Type {
		return 0, errors.New("cannot compare values of different types")
	}
	if a.Type == table.TypeStr {
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

func evalBinOp(op ExprOp, left, right *table.Cell) (*table.Cell, error) {
	switch op {
	case OP_ADD, OP_SUB, OP_MUL, OP_DIV:
		if left.Type != table.TypeI64 || right.Type != table.TypeI64 {
			return nil, errors.New("arithmetic requires integers")
		}
		switch op {
		case OP_ADD:
			return &table.Cell{Type: table.TypeI64, I64: left.I64 + right.I64}, nil
		case OP_SUB:
			return &table.Cell{Type: table.TypeI64, I64: left.I64 - right.I64}, nil
		case OP_MUL:
			return &table.Cell{Type: table.TypeI64, I64: left.I64 * right.I64}, nil
		default:
			if right.I64 == 0 {
				return nil, errors.New("division by zero")
			}
			return &table.Cell{Type: table.TypeI64, I64: left.I64 / right.I64}, nil
		}

	case OP_EQ, OP_NE:
		r, err := cmp(left, right)
		if err != nil {
			return nil, err
		}
		return boolCell((op == OP_EQ) == (r == 0)), nil

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

	case OP_AND:
		return boolCell(truthy(left) && truthy(right)), nil
	case OP_OR:
		return boolCell(truthy(left) || truthy(right)), nil
	}
	return nil, fmt.Errorf("unknown operator %d", op)
}
