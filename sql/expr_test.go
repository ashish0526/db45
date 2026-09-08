package sql

import (
	"testing"

	"github.com/ashish0526/db45/table"
)

func TestParseAddTree(t *testing.T) {
	// a + b - 3  =>  (a + b) - 3
	got, err := NewParser("a + b - 3").parseAdd()
	if err != nil {
		t.Fatal(err)
	}
	top, ok := got.(*ExprBinOp)
	if !ok || top.op != OP_SUB {
		t.Fatalf("top: %#v", got)
	}
	rc, ok := top.right.(*table.Cell)
	if !ok || rc.I64 != 3 {
		t.Fatalf("right: %#v", top.right)
	}
	lhs, ok := top.left.(*ExprBinOp)
	if !ok || lhs.op != OP_ADD || lhs.left != "a" || lhs.right != "b" {
		t.Fatalf("left subtree: %#v", top.left)
	}
}

func TestParsePrecedenceAndParens(t *testing.T) {
	schema := &table.Schema{Table: "t", Cols: []table.Column{{Name: "a", Type: table.TypeI64}}, PKey: []int{0}}
	row := table.Row{{Type: table.TypeI64, I64: 0}}
	eval := func(sql string) int64 {
		t.Helper()
		e, err := NewParser(sql).parseExpr()
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		c, err := evalExpr(schema, row, e)
		if err != nil {
			t.Fatalf("eval %q: %v", sql, err)
		}
		return c.I64
	}

	if got := eval("2 + 3 * 4"); got != 14 {
		t.Fatalf("2+3*4 = %d want 14", got)
	}
	if got := eval("(2 + 3) * 4"); got != 20 {
		t.Fatalf("(2+3)*4 = %d want 20", got)
	}
	if got := eval("20 / 2 / 5"); got != 2 { // left-assoc
		t.Fatalf("20/2/5 = %d want 2", got)
	}
}

func TestParseAtom(t *testing.T) {
	if v, _ := NewParser("col_x").parseAtom(); v != "col_x" {
		t.Fatalf("column atom: %#v", v)
	}
	v, err := NewParser("'lit'").parseAtom()
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := v.(*table.Cell); !ok || string(c.Str) != "lit" {
		t.Fatalf("literal atom: %#v", v)
	}
}
