package db

import (
	"reflect"
	"testing"
)

func TestParseFullGrammarAST(t *testing.T) {
	// f or e and not d = a + b * -c
	got, err := NewParser("f or e and not d = a + b * -c").parseExpr()
	if err != nil {
		t.Fatal(err)
	}
	want := &ExprBinOp{op: OP_OR, left: "f",
		right: &ExprBinOp{op: OP_AND, left: "e",
			right: &ExprUnOp{op: OP_NOT,
				kid: &ExprBinOp{op: OP_EQ, left: "d",
					right: &ExprBinOp{op: OP_ADD, left: "a",
						right: &ExprBinOp{op: OP_MUL, left: "b",
							right: &ExprUnOp{op: OP_NEG, kid: "c"}}}}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AST mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestEvalFullGrammar(t *testing.T) {
	schema := &Schema{
		Table: "t",
		Cols:  []Column{{"a", TypeI64}, {"b", TypeI64}},
		PKey:  []int{0},
	}
	row := Row{{Type: TypeI64, I64: 5}, {Type: TypeI64, I64: 2}}

	eval := func(sql string) *Cell {
		t.Helper()
		e, err := NewParser(sql).parseExpr()
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		c, err := evalExpr(schema, row, e)
		if err != nil {
			t.Fatalf("eval %q: %v", sql, err)
		}
		return c
	}

	if eval("a = 5 and b = 2").I64 != 1 {
		t.Fatal("a=5 and b=2")
	}
	if eval("a = 5 and b = 99").I64 != 0 {
		t.Fatal("a=5 and b=99")
	}
	if eval("a != b or b > 100").I64 != 1 {
		t.Fatal("a!=b or ...")
	}
	if eval("not (a < b)").I64 != 1 {
		t.Fatal("not (a<b)")
	}
	if eval("a * 2 + b").I64 != 12 {
		t.Fatal("a*2+b")
	}
	if eval("-a + 10").I64 != 5 {
		t.Fatal("-a+10")
	}
}
