package db

import "testing"

func TestParseSelect(t *testing.T) {
	p := NewParser("select a, b ,c from tbl where x = 1 and y = 'z'")
	var s StmtSelect
	if err := p.parseSelect(&s); err != nil {
		t.Fatalf("parseSelect: %v", err)
	}
	if s.table != "tbl" {
		t.Fatalf("table: %q", s.table)
	}
	if got := len(s.cols); got != 3 || s.cols[0] != "a" || s.cols[2] != "c" {
		t.Fatalf("cols: %v", s.cols)
	}
	if len(s.keys) != 2 {
		t.Fatalf("keys: %+v", s.keys)
	}
	if s.keys[0].column != "x" || s.keys[0].value.Type != TypeI64 || s.keys[0].value.I64 != 1 {
		t.Fatalf("key 0: %+v", s.keys[0])
	}
	if s.keys[1].column != "y" || string(s.keys[1].value.Str) != "z" {
		t.Fatalf("key 1: %+v", s.keys[1])
	}
}

func TestParseSelectNoWhere(t *testing.T) {
	p := NewParser("select id from t")
	var s StmtSelect
	if err := p.parseSelect(&s); err != nil {
		t.Fatalf("parseSelect: %v", err)
	}
	if len(s.keys) != 0 || s.table != "t" || len(s.cols) != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestParseSelectErrors(t *testing.T) {
	for _, in := range []string{
		"select from t",
		"select a b from t",
		"select a from",
		"update t",
	} {
		p := NewParser(in)
		var s StmtSelect
		if err := p.parseSelect(&s); err == nil {
			t.Fatalf("%q: expected error", in)
		}
	}
}
