package db

import (
	"bytes"
	"testing"
)

// create table link (time int64, src string, dst string, primary key (src, dst))
func linkSchema() *Schema {
	return &Schema{
		Table: "link",
		Cols: []Column{
			{Name: "time", Type: TypeI64},
			{Name: "src", Type: TypeStr},
			{Name: "dst", Type: TypeStr},
		},
		PKey: []int{1, 2},
	}
}

func TestRowKeyValRoundTrip(t *testing.T) {
	schema := linkSchema()

	row := schema.NewRow()
	row[0] = Cell{Type: TypeI64, I64: 1700000000}
	row[1] = Cell{Type: TypeStr, Str: []byte("alice")}
	row[2] = Cell{Type: TypeStr, Str: []byte("bob")}

	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)

	if !bytes.HasPrefix(key, []byte("link\x00")) {
		t.Fatalf("key missing table prefix: %q", key)
	}

	got := schema.NewRow()
	if err := got.DecodeKey(schema, key); err != nil {
		t.Fatalf("DecodeKey: %v", err)
	}
	if err := got.DecodeVal(schema, val); err != nil {
		t.Fatalf("DecodeVal: %v", err)
	}

	if got[0].I64 != row[0].I64 ||
		!bytes.Equal(got[1].Str, row[1].Str) ||
		!bytes.Equal(got[2].Str, row[2].Str) {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, row)
	}
}

func TestDecodeKeyWrongTable(t *testing.T) {
	schema := linkSchema()
	got := schema.NewRow()
	if err := got.DecodeKey(schema, []byte("other\x00stuff")); err != ErrOutOfRange {
		t.Fatalf("got %v want ErrOutOfRange", err)
	}
}

// "ab" and "abc" must not produce colliding key ranges.
func TestTablePrefixNoCollision(t *testing.T) {
	ab := (&Schema{Table: "ab", Cols: []Column{{"k", TypeStr}}, PKey: []int{0}}).NewRow()
	abc := (&Schema{Table: "abc", Cols: []Column{{"k", TypeStr}}, PKey: []int{0}}).NewRow()
	ab[0] = Cell{Type: TypeStr, Str: []byte("x")}
	abc[0] = Cell{Type: TypeStr, Str: []byte("x")}

	kAB := ab.EncodeKey(&Schema{Table: "ab"})
	kABC := abc.EncodeKey(&Schema{Table: "abc"})
	if bytes.HasPrefix(kABC, kAB) {
		t.Fatalf("table 'abc' key %q has 'ab' key %q as a prefix", kABC, kAB)
	}
}
