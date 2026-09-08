package db

import (
	"bytes"
	"io"
	"testing"
)

func TestEntryRoundTrip(t *testing.T) {
	cases := []Entry{
		{key: []byte("k1"), val: []byte("value one")},
		{key: []byte(""), val: []byte("")},
		{key: []byte("binary\x00\x01\xff"), val: []byte("\x00\x00")},
	}

	var buf bytes.Buffer
	for i := range cases {
		buf.Write(cases[i].Encode())
	}

	for i, want := range cases {
		var got Entry
		if err := got.Decode(&buf); err != nil {
			t.Fatalf("case %d Decode: %v", i, err)
		}
		if !bytes.Equal(got.key, want.key) || !bytes.Equal(got.val, want.val) {
			t.Fatalf("case %d: got {%q,%q} want {%q,%q}", i, got.key, got.val, want.key, want.val)
		}
	}

	// stream is now exactly exhausted
	var extra Entry
	if err := extra.Decode(&buf); err != io.EOF {
		t.Fatalf("expected io.EOF at clean end, got %v", err)
	}
}

func TestEntryTornRecord(t *testing.T) {
	ent := Entry{key: []byte("hello"), val: []byte("world")}
	full := ent.Encode()

	var got Entry
	if err := got.Decode(bytes.NewReader(full[:len(full)-3])); err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF for a truncated record, got %v", err)
	}
}
