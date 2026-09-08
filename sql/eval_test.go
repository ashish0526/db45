package sql

import (
	"testing"

	"github.com/ashish0526/db45/table"
)

func TestEvalArithmeticFromParser(t *testing.T) {
	schema := &table.Schema{
		Table: "t",
		Cols:  []table.Column{{Name: "a", Type: table.TypeI64}, {Name: "b", Type: table.TypeI64}, {Name: "s", Type: table.TypeStr}},
		PKey:  []int{0},
	}
	row := table.Row{
		{Type: table.TypeI64, I64: 10},
		{Type: table.TypeI64, I64: 3},
		{Type: table.TypeStr, Str: []byte("hi")},
	}
	eval := func(sql string) *table.Cell {
		t.Helper()
		e, err := NewParser(sql).parseAdd()
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		c, err := evalExpr(schema, row, e)
		if err != nil {
			t.Fatalf("eval %q: %v", sql, err)
		}
		return c
	}

	if c := eval("a - b - 2"); c.I64 != 5 { // left-assoc: (10-3)-2
		t.Fatalf("a-b-2 = %d want 5", c.I64)
	}
	if c := eval("a + 100"); c.I64 != 110 {
		t.Fatalf("a+100 = %d", c.I64)
	}
	if c := eval("s"); string(c.Str) != "hi" {
		t.Fatalf("column s = %+v", c)
	}
}

// Comparison ops aren't parseable until Step 0504; test evalExpr on hand-built trees.
func TestEvalComparisons(t *testing.T) {
	schema := &table.Schema{Table: "t", Cols: []table.Column{{Name: "a", Type: table.TypeI64}}, PKey: []int{0}}
	row := table.Row{{Type: table.TypeI64, I64: 10}}

	lit := func(n int64) *table.Cell { return &table.Cell{Type: table.TypeI64, I64: n} }
	for _, tc := range []struct {
		op   ExprOp
		want int64
	}{
		{OP_GT, 0}, {OP_GE, 1}, {OP_LT, 0}, {OP_LE, 1}, // a == 10
	} {
		e := &ExprBinOp{op: tc.op, left: "a", right: lit(10)}
		c, err := evalExpr(schema, row, e)
		if err != nil {
			t.Fatalf("op %d: %v", tc.op, err)
		}
		if c.Type != table.TypeI64 || c.I64 != tc.want {
			t.Fatalf("a %d 10 => %+v want %d", tc.op, c, tc.want)
		}
	}
}

func TestEvalExprErrors(t *testing.T) {
	schema := &table.Schema{Table: "t", Cols: []table.Column{{Name: "a", Type: table.TypeI64}}, PKey: []int{0}}
	row := table.Row{{Type: table.TypeI64, I64: 1}}
	for _, sql := range []string{"nope + 1", "a + a + missing"} {
		e, _ := NewParser(sql).parseAdd()
		if _, err := evalExpr(schema, row, e); err == nil {
			t.Errorf("%q: expected error", sql)
		}
	}
}
