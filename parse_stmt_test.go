package db

import "testing"

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
	if len(s.keys) != 2 || s.keys[0].column != "x" || s.keys[0].value.I64 != 1 {
		t.Fatalf("keys: %+v", s.keys)
	}
	if string(s.keys[1].value.Str) != "z" {
		t.Fatalf("key 1: %+v", s.keys[1])
	}
}

func TestParseCreateTable(t *testing.T) {
	s := mustParse(t,
		"create table link (time int64, src string, dst string, primary key (src, dst))").(*StmtCreatTable)
	if s.table != "link" || len(s.cols) != 3 {
		t.Fatalf("cols: %+v", s)
	}
	if s.cols[0].Type != TypeI64 || s.cols[1].Type != TypeStr {
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
	if c, ok := s.value[0].expr.(*Cell); !ok || c.I64 != 2 {
		t.Fatalf("set expr: %#v", s.value[0].expr)
	}
	if len(s.keys) != 2 {
		t.Fatalf("keys: %+v", s.keys)
	}
}

func TestParseDelete(t *testing.T) {
	s := mustParse(t, "delete from link where src = 'a' and dst = 'b'").(*StmtDelete)
	if s.table != "link" || len(s.keys) != 2 {
		t.Fatalf("got %+v", s)
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
