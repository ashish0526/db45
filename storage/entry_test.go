package storage

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
		{key: []byte("gone"), deleted: true},
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
		if !bytes.Equal(got.key, want.key) || !bytes.Equal(got.val, want.val) || got.deleted != want.deleted {
			t.Fatalf("case %d: got %+v want %+v", i, got, want)
		}
	}

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
		t.Fatalf("truncated body: got %v want io.ErrUnexpectedEOF", err)
	}
	if err := got.Decode(bytes.NewReader(full[:6])); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated header: got %v want io.ErrUnexpectedEOF", err)
	}
}

func TestEntryBadChecksum(t *testing.T) {
	ent := Entry{key: []byte("hello"), val: []byte("world")}
	full := ent.Encode()
	full[len(full)-1] ^= 0xff // flip a byte in the value

	var got Entry
	if err := got.Decode(bytes.NewReader(full)); err != ErrBadSum {
		t.Fatalf("corrupted value: got %v want ErrBadSum", err)
	}
}
