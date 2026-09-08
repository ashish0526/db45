package table

import (
	"bytes"
	"testing"
)

func openDB(t *testing.T) *DB {
	t.Helper()
	return openDBDir(t, t.TempDir())
}

func openDBDir(t *testing.T, dir string) *DB {
	t.Helper()
	db := &DB{}
	db.KV.Options.Dirpath = dir
	if err := db.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mkRow(schema *Schema, tm int64, src, dst string) Row {
	r := schema.NewRow()
	r[0] = Cell{Type: TypeI64, I64: tm}
	r[1] = Cell{Type: TypeStr, Str: []byte(src)}
	r[2] = Cell{Type: TypeStr, Str: []byte(dst)}
	return r
}

func TestDBCRUD(t *testing.T) {
	db := openDB(t)
	schema := linkSchema()

	// Insert
	if ok, err := db.Insert(schema, mkRow(schema, 100, "a", "b")); err != nil || !ok {
		t.Fatalf("Insert: ok=%v err=%v", ok, err)
	}
	// duplicate Insert is a no-op
	if ok, _ := db.Insert(schema, mkRow(schema, 999, "a", "b")); ok {
		t.Fatal("duplicate Insert reported a write")
	}

	// Select by primary key
	got := schema.NewRow()
	got[1] = Cell{Type: TypeStr, Str: []byte("a")}
	got[2] = Cell{Type: TypeStr, Str: []byte("b")}
	if ok, err := db.Select(schema, got); err != nil || !ok {
		t.Fatalf("Select: ok=%v err=%v", ok, err)
	}
	if got[0].I64 != 100 {
		t.Fatalf("Select time: got %d want 100", got[0].I64)
	}

	// Update changes the value
	if ok, err := db.Update(schema, mkRow(schema, 200, "a", "b")); err != nil || !ok {
		t.Fatalf("Update: ok=%v err=%v", ok, err)
	}
	if ok, _ := db.Update(schema, mkRow(schema, 1, "no", "row")); ok {
		t.Fatal("Update of absent row reported a write")
	}
	db.Select(schema, got)
	if got[0].I64 != 200 {
		t.Fatalf("after Update: got %d want 200", got[0].I64)
	}

	// Upsert both overwrites and inserts
	if _, err := db.Upsert(schema, mkRow(schema, 300, "a", "b")); err != nil {
		t.Fatalf("Upsert existing: %v", err)
	}
	if _, err := db.Upsert(schema, mkRow(schema, 400, "c", "d")); err != nil {
		t.Fatalf("Upsert new: %v", err)
	}

	// Delete
	del := schema.NewRow()
	del[1] = Cell{Type: TypeStr, Str: []byte("a")}
	del[2] = Cell{Type: TypeStr, Str: []byte("b")}
	if ok, err := db.Delete(schema, del); err != nil || !ok {
		t.Fatalf("Delete: ok=%v err=%v", ok, err)
	}
	if ok, _ := db.Select(schema, del); ok {
		t.Fatal("row still present after Delete")
	}

	// the (c,d) row is untouched
	cd := schema.NewRow()
	cd[1] = Cell{Type: TypeStr, Str: []byte("c")}
	cd[2] = Cell{Type: TypeStr, Str: []byte("d")}
	if ok, _ := db.Select(schema, cd); !ok || cd[0].I64 != 400 || !bytes.Equal(cd[1].Str, []byte("c")) {
		t.Fatalf("(c,d) row: ok=%v row=%+v", ok, cd)
	}
}
