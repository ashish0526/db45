package table

import "testing"

func TestRowIteratorScanStopsAtTableBoundary(t *testing.T) {
	db := openDB(t)
	link := linkSchema()

	for _, r := range []Row{
		mkRow(link, 1, "a", "b"),
		mkRow(link, 2, "a", "c"),
		mkRow(link, 3, "d", "e"),
	} {
		if _, err := db.Insert(link, r); err != nil {
			t.Fatal(err)
		}
	}

	// a row in another table, sorting after link's keyspace
	other := &Schema{Table: "zztop", Cols: []Column{{"k", TypeStr}}, PKey: []int{0}}
	orow := other.NewRow()
	orow[0] = Cell{Type: TypeStr, Str: []byte("x")}
	db.Insert(other, orow)

	it, err := db.Scan(link)
	if err != nil {
		t.Fatal(err)
	}
	var seen [][3]string
	for ; it.Valid(); it.Next() {
		r := it.Row()
		seen = append(seen, [3]string{string(r[1].Str), string(r[2].Str), ""})
		_ = r[0].I64
	}
	if len(seen) != 3 {
		t.Fatalf("scanned %d rows, want 3: %v", len(seen), seen)
	}
	if seen[0][0] != "a" || seen[0][1] != "b" || seen[2][0] != "d" {
		t.Fatalf("scan order wrong: %v", seen)
	}
}

func TestRowIteratorSeekMidTable(t *testing.T) {
	db := openDB(t)
	link := linkSchema()
	for _, r := range []Row{
		mkRow(link, 1, "a", "b"),
		mkRow(link, 2, "m", "n"),
		mkRow(link, 3, "x", "y"),
	} {
		db.Insert(link, r)
	}

	start := mkRow(link, 0, "m", "").EncodeKey(link)
	it, err := db.Seek(link, start)
	if err != nil {
		t.Fatal(err)
	}
	if !it.Valid() || string(it.Row()[1].Str) != "m" {
		t.Fatalf("seek landed on %+v", it.Row())
	}
}
