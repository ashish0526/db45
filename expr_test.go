package db

import "testing"

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
	rc, ok := top.right.(*Cell)
	if !ok || rc.I64 != 3 {
		t.Fatalf("right: %#v", top.right)
	}
	lhs, ok := top.left.(*ExprBinOp)
	if !ok || lhs.op != OP_ADD || lhs.left != "a" || lhs.right != "b" {
		t.Fatalf("left subtree: %#v", top.left)
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
	if c, ok := v.(*Cell); !ok || string(c.Str) != "lit" {
		t.Fatalf("literal atom: %#v", v)
	}
}
