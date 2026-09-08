package table

import (
	"bytes"
	"testing"
)

func TestCellValRoundTrip(t *testing.T) {
	cells := []Cell{
		{Type: TypeI64, I64: 0},
		{Type: TypeI64, I64: -1},
		{Type: TypeI64, I64: 1<<63 - 1},
		{Type: TypeI64, I64: -1 << 63},
		{Type: TypeStr, Str: []byte("")},
		{Type: TypeStr, Str: []byte("hello \x00 world")},
	}

	var buf []byte
	for i := range cells {
		buf = cells[i].EncodeVal(buf)
	}
	rest := buf
	for i, want := range cells {
		got := Cell{Type: want.Type}
		var err error
		if rest, err = got.DecodeVal(rest); err != nil {
			t.Fatalf("cell %d DecodeVal: %v", i, err)
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

func TestCellKeyRoundTrip(t *testing.T) {
	for _, c := range []Cell{
		{Type: TypeI64, I64: 0},
		{Type: TypeI64, I64: -9999},
		{Type: TypeI64, I64: 1 << 40},
		{Type: TypeStr, Str: []byte("")},
		{Type: TypeStr, Str: []byte("a\x00b\x01c")},
	} {
		enc := c.EncodeKey(nil)
		got := Cell{Type: c.Type}
		rest, err := got.DecodeKey(enc)
		if err != nil || len(rest) != 0 {
			t.Fatalf("%+v: DecodeKey err=%v rest=%d", c, err, len(rest))
		}
		if c.Type == TypeI64 && got.I64 != c.I64 {
			t.Fatalf("i64 %d -> %d", c.I64, got.I64)
		}
		if c.Type == TypeStr && !bytes.Equal(got.Str, c.Str) {
			t.Fatalf("str %q -> %q", c.Str, got.Str)
		}
	}
}

// byte order of EncodeKey must equal logical order.
func TestCellKeyOrderPreserving(t *testing.T) {
	ints := []int64{-1 << 63, -1000, -1, 0, 1, 1000, 1<<63 - 1}
	for i := 0; i+1 < len(ints); i++ {
		a := (&Cell{Type: TypeI64, I64: ints[i]}).EncodeKey(nil)
		b := (&Cell{Type: TypeI64, I64: ints[i+1]}).EncodeKey(nil)
		if bytes.Compare(a, b) >= 0 {
			t.Fatalf("i64 order: %d not < %d (bytes %x vs %x)", ints[i], ints[i+1], a, b)
		}
	}

	strs := []string{"", "a", "ab", "abc", "b", "b\x00", "c"}
	for i := 0; i+1 < len(strs); i++ {
		a := (&Cell{Type: TypeStr, Str: []byte(strs[i])}).EncodeKey(nil)
		b := (&Cell{Type: TypeStr, Str: []byte(strs[i+1])}).EncodeKey(nil)
		if bytes.Compare(a, b) >= 0 {
			t.Fatalf("str order: %q not < %q", strs[i], strs[i+1])
		}
	}
}

// concatenated multi-column keys compare as tuples.
func TestMultiColumnKeyTupleOrder(t *testing.T) {
	mk := func(x int64, s string) []byte {
		out := (&Cell{Type: TypeI64, I64: x}).EncodeKey(nil)
		return (&Cell{Type: TypeStr, Str: []byte(s)}).EncodeKey(out)
	}
	pairs := [][2]interface{}{
		{int64(1), "a"}, {int64(1), "b"}, {int64(2), "a"}, {int64(2), "aa"}, {int64(10), ""},
	}
	for i := 0; i+1 < len(pairs); i++ {
		a := mk(pairs[i][0].(int64), pairs[i][1].(string))
		b := mk(pairs[i+1][0].(int64), pairs[i+1][1].(string))
		if bytes.Compare(a, b) >= 0 {
			t.Fatalf("tuple order broke at %v / %v", pairs[i], pairs[i+1])
		}
	}
}
