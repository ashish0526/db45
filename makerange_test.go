package db

import "testing"

func TestSQLRangeQueries(t *testing.T) {
	db := openDB(t)
	db.Exec("create table t (a int64, v string, primary key (a))")
	for i := 1; i <= 5; i++ {
		db.Exec("insert into t values (" + itoa(i) + ", 'x')")
	}

	get := func(sql string) []int64 {
		t.Helper()
		r, err := db.Exec(sql)
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		var out []int64
		for _, row := range r.Values {
			out = append(out, row[0].I64)
		}
		return out
	}

	if got := get("select a from t where a > 2"); !eqI64(got, []int64{3, 4, 5}) {
		t.Fatalf("a > 2: %v", got)
	}
	if got := get("select a from t where a >= 2 and a < 5"); !eqI64(got, []int64{2, 3, 4}) {
		t.Fatalf("a>=2 and a<5: %v", got)
	}
	if got := get("select a from t where a < 3"); !eqI64(got, []int64{1, 2}) {
		t.Fatalf("a < 3: %v", got)
	}
	if got := get("select a from t"); !eqI64(got, []int64{1, 2, 3, 4, 5}) {
		t.Fatalf("full scan: %v", got)
	}
	if got := get("select a from t where a = 3"); !eqI64(got, []int64{3}) {
		t.Fatalf("point: %v", got)
	}
}

func TestSQLRangeOnCompositePrefix(t *testing.T) {
	db := openDB(t)
	db.Exec("create table e (a int64, b int64, primary key (a, b))")
	db.Exec("insert into e values (1, 1)")
	db.Exec("insert into e values (1, 2)")
	db.Exec("insert into e values (2, 1)")
	db.Exec("insert into e values (3, 9)")

	r, err := db.Exec("select a, b from e where (a, b) > (1, 1)")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Values) != 3 {
		t.Fatalf("(a,b) > (1,1): %+v", r.Values)
	}
	// first result is (1,2)
	if r.Values[0][0].I64 != 1 || r.Values[0][1].I64 != 2 {
		t.Fatalf("first row: %+v", r.Values[0])
	}
}

func TestSQLMultiRowUpdateDelete(t *testing.T) {
	db := openDB(t)
	db.Exec("create table t (a int64, v int64, primary key (a))")
	for i := 1; i <= 5; i++ {
		db.Exec("insert into t values (" + itoa(i) + ", 0)")
	}

	r, err := db.Exec("update t set v = a * 10 where a >= 3")
	if err != nil || r.Updated != 3 {
		t.Fatalf("update: r=%+v err=%v", r, err)
	}
	sel, _ := db.Exec("select v from t where a = 4")
	if sel.Values[0][0].I64 != 40 {
		t.Fatalf("updated value: %+v", sel.Values[0])
	}

	r, err = db.Exec("delete from t where a < 3")
	if err != nil || r.Updated != 2 {
		t.Fatalf("delete: r=%+v err=%v", r, err)
	}
	all, _ := db.Exec("select a from t")
	if len(all.Values) != 3 {
		t.Fatalf("remaining: %+v", all.Values)
	}
}

// tiny helpers
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func eqI64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
