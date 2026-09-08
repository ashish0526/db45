package sql

import (
	"testing"

	"github.com/ashish0526/db45/table"
)

func mustParse(t *testing.T, sql string) interface{} {
	t.Helper()
	stmt, err := NewParser(sql).ParseStmt()
	if err != nil {
		t.Fatalf("ParseStmt(%q): %v", sql, err)
	}
	return stmt
}

func TestParseSelect(t *testing.T) {
	s := mustParse(t, "select a, b ,c from tbl where x = 1 and y = 'z';").(*StmtSelect)
	if s.table != "tbl" || len(s.cols) != 3 || s.cols[2] != "c" {
		t.Fatalf("got %+v", s)
	}
	// WHERE is now an expression tree: (x = 1) AND (y = 'z')
	keys, ok := matchAllEq(s.cond, nil)
	if !ok || len(keys) != 2 || keys[0].column != "x" || keys[0].value.I64 != 1 {
		t.Fatalf("cond: %#v", s.cond)
	}
	if string(keys[1].value.Str) != "z" {
		t.Fatalf("key 1: %+v", keys[1])
	}
}

func TestParseCreateTable(t *testing.T) {
	s := mustParse(t,
		"create table link (time int64, src string, dst string, primary key (src, dst))").(*StmtCreatTable)
	if s.table != "link" || len(s.cols) != 3 {
		t.Fatalf("cols: %+v", s)
	}
	if s.cols[0].Type != table.TypeI64 || s.cols[1].Type != table.TypeStr {
		t.Fatalf("types: %+v", s.cols)
	}
	if len(s.pkey) != 2 || s.pkey[0] != "src" || s.pkey[1] != "dst" {
		t.Fatalf("pkey: %v", s.pkey)
	}
}

func TestParseInsert(t *testing.T) {
	s := mustParse(t, "insert into link values (1700, 'a', 'b')").(*StmtInsert)
	if s.table != "link" || len(s.value) != 3 {
		t.Fatalf("got %+v", s)
	}
	if s.value[0].I64 != 1700 || string(s.value[1].Str) != "a" {
		t.Fatalf("values: %+v", s.value)
	}
}

func TestParseUpdate(t *testing.T) {
	s := mustParse(t, "update link set time = 2 where src = 'a' and dst = 'b'").(*StmtUpdate)
	if s.table != "link" || len(s.value) != 1 || s.value[0].column != "time" {
		t.Fatalf("set: %+v", s.value)
	}
	if c, ok := s.value[0].expr.(*table.Cell); !ok || c.I64 != 2 {
		t.Fatalf("set expr: %#v", s.value[0].expr)
	}
	if keys, ok := matchAllEq(s.cond, nil); !ok || len(keys) != 2 {
		t.Fatalf("cond: %#v", s.cond)
	}
}

func TestParseDelete(t *testing.T) {
	s := mustParse(t, "delete from link where src = 'a' and dst = 'b'").(*StmtDelete)
	if keys, ok := matchAllEq(s.cond, nil); s.table != "link" || !ok || len(keys) != 2 {
		t.Fatalf("got %+v cond %#v", s, s.cond)
	}
}

func TestParseStmtErrors(t *testing.T) {
	for _, in := range []string{
		"select from t",
		"select a b from t",
		"frobnicate t",
		"create table t (a int64)", // no primary key
		"insert into t values ()",
		"update t set a where k = 1",
		"select a from t extra junk",
	} {
		if _, err := NewParser(in).ParseStmt(); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}
