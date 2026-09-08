package db

import "testing"

func kvSchema() *Schema {
	return &Schema{
		Table: "kvs",
		Cols:  []Column{{"k", TypeStr}, {"v", TypeStr}},
		PKey:  []int{0},
	}
}

func seedKVS(t *testing.T, db *DB, keys ...string) *Schema {
	t.Helper()
	s := kvSchema()
	db.KV.Set([]byte(schemaKeyPrefix+"kvs"), []byte("{}"))
	for _, k := range keys {
		r := s.NewRow()
		r[0] = Cell{Type: TypeStr, Str: []byte(k)}
		r[1] = Cell{Type: TypeStr, Str: []byte(k + "-val")}
		if _, err := db.Insert(s, r); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func collectKeys(t *testing.T, it *RowIterator) []string {
	t.Helper()
	var out []string
	for ; it.Valid(); it.Next() {
		out = append(out, string(it.Row()[0].Str))
	}
	return out
}

func str(s string) []Cell { return []Cell{{Type: TypeStr, Str: []byte(s)}} }

func TestDBRangeAscendingDescending(t *testing.T) {
	db := openDB(t)
	s := seedKVS(t, db, "a", "b", "c", "d", "e")

	asc, err := db.Range(s, &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE, Start: str("b"), Stop: str("d")})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectKeys(t, asc); len(got) != 3 || got[0] != "b" || got[2] != "d" {
		t.Fatalf("ascending closed range: %v", got)
	}

	desc, err := db.Range(s, &RangeReq{StartCmp: OP_LE, StopCmp: OP_GE, Start: str("d"), Stop: str("b")})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectKeys(t, desc); len(got) != 3 || got[0] != "d" || got[2] != "b" {
		t.Fatalf("descending range: %v", got)
	}
}

func TestDBRangeOpenEnded(t *testing.T) {
	db := openDB(t)
	s := seedKVS(t, db, "a", "b", "c", "d", "e")

	// k > "c"  == full-key >= (c, +inf)
	gt, _ := db.Range(s, &RangeReq{StartCmp: OP_GT, StopCmp: OP_LE, Start: str("c"), Stop: nil})
	if got := collectKeys(t, gt); len(got) != 2 || got[0] != "d" || got[1] != "e" {
		t.Fatalf("k > c: %v", got)
	}

	// full scan: -inf .. +inf
	all, _ := db.Range(s, &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE, Start: nil, Stop: nil})
	if got := collectKeys(t, all); len(got) != 5 {
		t.Fatalf("full scan: %v", got)
	}
}

func TestDBRangePrefixOnCompositeKey(t *testing.T) {
	db := openDB(t)
	link := linkSchema()
	db.KV.Set([]byte(schemaKeyPrefix+"link"), []byte("{}"))
	for _, r := range []Row{
		mkRow(link, 1, "a", "m"),
		mkRow(link, 2, "a", "n"),
		mkRow(link, 3, "b", "x"),
		mkRow(link, 4, "c", "y"),
	} {
		db.Insert(link, r)
	}

	// WHERE src = 'a'  -> prefix range (a) <= (src,dst) <= (a)
	it, err := db.Range(link, &RangeReq{
		StartCmp: OP_GE, StopCmp: OP_LE,
		Start: str("a"), Stop: str("a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for ; it.Valid(); it.Next() {
		got = append(got, string(it.Row()[2].Str))
	}
	if len(got) != 2 || got[0] != "m" || got[1] != "n" {
		t.Fatalf("prefix src='a': %v", got)
	}
}
