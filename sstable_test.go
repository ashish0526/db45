package db

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// memSorted is a tiny in-memory SortedKV for tests, backed by a SortedArray.
type memSorted struct{ a SortedArray }

func newMem(pairs ...[2]string) *memSorted {
	m := &memSorted{}
	for _, p := range pairs {
		m.a.Push([]byte(p[0]), []byte(p[1]), false)
	}
	return m
}
func (m *memSorted) tomb(key string) { m.a.Push([]byte(key), nil, true) }

func (m *memSorted) EstimatedSize() int                  { return m.a.EstimatedSize() }
func (m *memSorted) Iter() (SortedKVIter, error)         { return m.a.Iter() }
func (m *memSorted) Seek(k []byte) (SortedKVIter, error) { return m.a.Seek(k) }

func TestSortedFileCreateLayout(t *testing.T) {
	want := [][2]string{{"a", "1"}, {"bb", "22"}, {"ccc", ""}}
	src := newMem(want...)
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
	for i := 0; i < 3; i++ {
		off := binary.LittleEndian.Uint64(raw[8+8*i : 16+8*i])
		klen := binary.LittleEndian.Uint32(raw[off : off+4])
		vlen := binary.LittleEndian.Uint32(raw[off+4 : off+8])
		key := raw[off+8 : off+8+uint64(klen)]
		val := raw[off+8+uint64(klen) : off+8+uint64(klen)+uint64(vlen)]
		if string(key) != want[i][0] || string(val) != want[i][1] {
			t.Fatalf("record %d: {%q,%q} want %v", i, key, val, want[i])
		}
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
