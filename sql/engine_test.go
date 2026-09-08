package sql

import "testing"

func TestExecEndToEnd(t *testing.T) {
	db := openDB(t)

	mustExec := func(sql string) SQLResult {
		t.Helper()
		r, err := db.Exec(sql)
		if err != nil {
			t.Fatalf("Exec(%q): %v", sql, err)
		}
		return r
	}

	mustExec("create table link (time int64, src string, dst string, primary key (src, dst))")

	if r := mustExec("insert into link values (100, 'a', 'b')"); r.Updated != 1 {
		t.Fatalf("insert Updated=%d", r.Updated)
	}
	mustExec("insert into link values (200, 'c', 'd')")

	// SELECT projects to the requested columns
	r := mustExec("select time, dst from link where src = 'a' and dst = 'b'")
	if len(r.Values) != 1 || len(r.Values[0]) != 2 {
		t.Fatalf("select shape: %+v", r)
	}
	if r.Values[0][0].I64 != 100 || string(r.Values[0][1].Str) != "b" {
		t.Fatalf("select values: %+v", r.Values[0])
	}

	// UPDATE only rewrites V
	if r := mustExec("update link set time = 999 where src = 'a' and dst = 'b'"); r.Updated != 1 {
		t.Fatalf("update Updated=%d", r.Updated)
	}
	r = mustExec("select time from link where src = 'a' and dst = 'b'")
	if r.Values[0][0].I64 != 999 {
		t.Fatalf("after update: %d", r.Values[0][0].I64)
	}

	// DELETE
	if r := mustExec("delete from link where src = 'a' and dst = 'b'"); r.Updated != 1 {
		t.Fatalf("delete Updated=%d", r.Updated)
	}
	if r := mustExec("select time from link where src = 'a' and dst = 'b'"); len(r.Values) != 0 {
		t.Fatalf("row survived delete: %+v", r.Values)
	}
}

func TestExecExpressionsInSelectAndUpdate(t *testing.T) {
	db := openDB(t)
	db.Exec("create table t (id int64, a int64, b int64, primary key (id))")
	db.Exec("insert into t values (1, 10, 3)")

	r, err := db.Exec("select a * 4 - b, b + a, id from t where id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Values) != 1 || r.Values[0][0].I64 != 37 || r.Values[0][1].I64 != 13 || r.Values[0][2].I64 != 1 {
		t.Fatalf("select exprs: %+v", r.Values)
	}
	if r.Header[0] != "col1" || r.Header[2] != "id" {
		t.Fatalf("header: %v", r.Header)
	}

	// SET a = b, b = a must swap (both RHS see the old row)
	if _, err := db.Exec("update t set a = b, b = a where id = 1"); err != nil {
		t.Fatal(err)
	}
	r, _ = db.Exec("select a, b from t where id = 1")
	if r.Values[0][0].I64 != 3 || r.Values[0][1].I64 != 10 {
		t.Fatalf("swap failed: %+v", r.Values[0])
	}
}

func TestExecCatalogPersists(t *testing.T) {
	dir := t.TempDir()

	db1 := openDBDir(t, dir)
	db1.Exec("create table t (id int64, name string, primary key (id))")
	db1.Exec("insert into t values (1, 'one')")
	db1.Close()

	// reopen: schema must be recoverable from the catalog alone
	db2 := openDBDir(t, dir)

	r, err := db2.Exec("select name from t where id = 1")
	if err != nil {
		t.Fatalf("select after reopen: %v", err)
	}
	if len(r.Values) != 1 || string(r.Values[0][0].Str) != "one" {
		t.Fatalf("got %+v", r.Values)
	}
}

func TestExecErrors(t *testing.T) {
	db := openDB(t)
	db.Exec("create table t (id int64, v string, primary key (id))")

	for _, sql := range []string{
		"select v from missing where id = 1",
		"select nope from t where id = 1",
		"select v from t where v = 'x'", // WHERE not on the primary key
		"insert into t values (1)",      // wrong arity
	} {
		if _, err := db.Exec(sql); err == nil {
			t.Errorf("%q: expected error", sql)
		}
	}

	db.Exec("insert into t values (1, 'a')")
	if _, err := db.Exec("insert into t values (1, 'b')"); err == nil {
		t.Error("duplicate insert should error")
	}

	// OR is still not a recognised access path
	if _, err := db.Exec("select v from t where id = 1 or id = 2"); err == nil {
		t.Error("OR WHERE should be unimplemented")
	}
}

func TestExecWhereExpressionShape(t *testing.T) {
	db := openDB(t)
	db.Exec("create table t (a int64, b int64, v string, primary key (a, b))")
	db.Exec("insert into t values (1, 2, 'hit')")

	// AND is commutative for the matcher
	r, err := db.Exec("select v from t where b = 2 and a = 1")
	if err != nil || len(r.Values) != 1 || string(r.Values[0][0].Str) != "hit" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
}
