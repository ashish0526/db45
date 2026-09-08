package sql

import (
	"testing"

	"github.com/ashish0526/db45/table"
)

func kvSchema() *table.Schema {
	return &table.Schema{
		Table: "kvs",
		Cols:  []table.Column{{Name: "k", Type: table.TypeStr}, {Name: "v", Type: table.TypeStr}},
		PKey:  []int{0},
	}
}

func seedKVS(t *testing.T, e *Engine, keys ...string) *table.Schema {
	t.Helper()
	s := kvSchema()
	for _, k := range keys {
		r := s.NewRow()
		r[0] = table.Cell{Type: table.TypeStr, Str: []byte(k)}
		r[1] = table.Cell{Type: table.TypeStr, Str: []byte(k + "-val")}
		if _, err := e.DB.Insert(s, r); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func collectKeys(t *testing.T, it *table.RowIterator) []string {
	t.Helper()
	var out []string
	for ; it.Valid(); it.Next() {
		out = append(out, string(it.Row()[0].Str))
	}
	return out
}

func str(s string) []table.Cell { return []table.Cell{{Type: table.TypeStr, Str: []byte(s)}} }

func TestRangeAscendingDescending(t *testing.T) {
	e := openDB(t)
	s := seedKVS(t, e, "a", "b", "c", "d", "e")

	asc, err := Range(e.DB, s, &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE, Start: str("b"), Stop: str("d")})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectKeys(t, asc); len(got) != 3 || got[0] != "b" || got[2] != "d" {
		t.Fatalf("ascending closed range: %v", got)
	}

	desc, err := Range(e.DB, s, &RangeReq{StartCmp: OP_LE, StopCmp: OP_GE, Start: str("d"), Stop: str("b")})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectKeys(t, desc); len(got) != 3 || got[0] != "d" || got[2] != "b" {
		t.Fatalf("descending range: %v", got)
	}
}

func TestRangeOpenEnded(t *testing.T) {
	e := openDB(t)
	s := seedKVS(t, e, "a", "b", "c", "d", "e")

	gt, _ := Range(e.DB, s, &RangeReq{StartCmp: OP_GT, StopCmp: OP_LE, Start: str("c"), Stop: nil})
	if got := collectKeys(t, gt); len(got) != 2 || got[0] != "d" || got[1] != "e" {
		t.Fatalf("k > c: %v", got)
	}

	all, _ := Range(e.DB, s, &RangeReq{StartCmp: OP_GE, StopCmp: OP_LE, Start: nil, Stop: nil})
	if got := collectKeys(t, all); len(got) != 5 {
		t.Fatalf("full scan: %v", got)
	}
}

func TestRangePrefixOnCompositeKey(t *testing.T) {
	e := openDB(t)
	link := linkSchema()
	for _, r := range []table.Row{
		mkRow(link, 1, "a", "m"),
		mkRow(link, 2, "a", "n"),
		mkRow(link, 3, "b", "x"),
		mkRow(link, 4, "c", "y"),
	} {
		e.DB.Insert(link, r)
	}

	it, err := Range(e.DB, link, &RangeReq{
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
