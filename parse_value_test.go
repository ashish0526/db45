package db

import "testing"

func TestParseValueInt(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"0", 0}, {"42", 42}, {"-7", -7}, {"+9", 9},
		{"9223372036854775807", 1<<63 - 1},
	} {
		var c Cell
		p := NewParser(tc.in)
		if err := p.parseValue(&c); err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if c.Type != TypeI64 || c.I64 != tc.want {
			t.Fatalf("%q: got %+v want %d", tc.in, c, tc.want)
		}
	}
}

func TestParseValueString(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{`'hello'`, "hello"},
		{`"world"`, "world"},
		{`"isn\'t"`, "isn't"},
		{`'a\\b'`, `a\b`},
		{`''`, ""},
	} {
		var c Cell
		p := NewParser(tc.in)
		if err := p.parseValue(&c); err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if c.Type != TypeStr || string(c.Str) != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, c.Str, tc.want)
		}
	}
}

func TestParseValueErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "'unterminated", `'bad\escape'`} {
		var c Cell
		p := NewParser(in)
		if err := p.parseValue(&c); err == nil {
			t.Fatalf("%q: expected an error", in)
		}
	}
}
