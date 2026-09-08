package db

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// memSorted is a tiny in-memory SortedKV for testing the file builder.
type memSorted struct {
	keys [][]byte
	vals [][]byte
}

func (m *memSorted) Size() int { return len(m.keys) }
func (m *memSorted) Iter() (SortedKVIter, error) {
	return &SortedArrayIter{keys: m.keys, vals: m.vals, pos: 0}, nil
}

func TestSortedFileCreateLayout(t *testing.T) {
	src := &memSorted{
		keys: [][]byte{[]byte("a"), []byte("bb"), []byte("ccc")},
		vals: [][]byte{[]byte("1"), []byte("22"), []byte("")},
	}
	path := filepath.Join(t.TempDir(), "sst")
	f := &SortedFile{FileName: path}
	if err := f.CreateFromSorted(src); err != nil {
		t.Fatalf("CreateFromSorted: %v", err)
	}
	f.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := binary.LittleEndian.Uint64(raw[0:8]); n != 3 {
		t.Fatalf("header n = %d want 3", n)
	}

	// walk the offset array and decode each record
	for i := 0; i < 3; i++ {
		off := binary.LittleEndian.Uint64(raw[8+8*i : 16+8*i])
		klen := binary.LittleEndian.Uint32(raw[off : off+4])
		vlen := binary.LittleEndian.Uint32(raw[off+4 : off+8])
		key := raw[off+8 : off+8+uint64(klen)]
		val := raw[off+8+uint64(klen) : off+8+uint64(klen)+uint64(vlen)]
		if string(key) != string(src.keys[i]) || string(val) != string(src.vals[i]) {
			t.Fatalf("record %d: {%q,%q} want {%q,%q}", i, key, val, src.keys[i], src.vals[i])
		}
	}

	// offsets must be strictly increasing and start after the header+index
	first := binary.LittleEndian.Uint64(raw[8:16])
	if first != uint64(8+8*3) {
		t.Fatalf("first offset = %d want %d", first, 8+8*3)
	}
}

func TestSortedFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sst")
	f := &SortedFile{FileName: path}
	if err := f.CreateFromSorted(&memSorted{}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	raw, _ := os.ReadFile(path)
	if len(raw) != 8 || binary.LittleEndian.Uint64(raw) != 0 {
		t.Fatalf("empty sstable = %v", raw)
	}
}
