package db

import (
	"bytes"
	"testing"
)

func TestCellRoundTrip(t *testing.T) {
	cells := []Cell{
		{Type: TypeI64, I64: 0},
		{Type: TypeI64, I64: -1},
		{Type: TypeI64, I64: 1<<63 - 1},
		{Type: TypeI64, I64: -1 << 63},
		{Type: TypeStr, Str: []byte("")},
		{Type: TypeStr, Str: []byte("hello \x00 world")},
	}

	// Encode them all into one buffer, then decode back in order.
	var buf []byte
	for i := range cells {
		buf = cells[i].Encode(buf)
	}

	rest := buf
	for i, want := range cells {
		got := Cell{Type: want.Type}
		var err error
		rest, err = got.Decode(rest)
		if err != nil {
			t.Fatalf("cell %d Decode: %v", i, err)
		}
		if got.Type == TypeI64 && got.I64 != want.I64 {
			t.Fatalf("cell %d: I64 got %d want %d", i, got.I64, want.I64)
		}
		if got.Type == TypeStr && !bytes.Equal(got.Str, want.Str) {
			t.Fatalf("cell %d: Str got %q want %q", i, got.Str, want.Str)
		}
	}
	if len(rest) != 0 {
		t.Fatalf("%d bytes left over", len(rest))
	}
}

func TestCellDecodeTruncated(t *testing.T) {
	full := (&Cell{Type: TypeStr, Str: []byte("abcdef")}).Encode(nil)
	got := Cell{Type: TypeStr}
	if _, err := got.Decode(full[:len(full)-2]); err != errShortCell {
		t.Fatalf("got %v want errShortCell", err)
	}
}
