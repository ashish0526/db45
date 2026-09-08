package storage

import (
	"path/filepath"
	"testing"
)

func buildSST(t *testing.T, pairs [][2]string) *SortedFile {
	t.Helper()
	src := newMem(pairs...)
	f := &SortedFile{FileName: filepath.Join(t.TempDir(), "sst")}
	if err := f.CreateFromSorted(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	g := &SortedFile{FileName: f.FileName}
	if err := g.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestSortedFileIndexAndSearch(t *testing.T) {
	f := buildSST(t, [][2]string{{"a", "1"}, {"c", "3"}, {"e", "5"}, {"g", "7"}})

	if f.Size() != 4 {
		t.Fatalf("Size = %d", f.Size())
	}
	k, v, del, err := f.index(2)
	if err != nil || string(k) != "e" || string(v) != "5" || del {
		t.Fatalf("index(2) = %q,%q,%v,%v", k, v, del, err)
	}

	pos, found, _ := f.search([]byte("c"))
	if !found || pos != 1 {
		t.Fatalf("search c = %d,%v", pos, found)
	}
	pos, found, _ = f.search([]byte("d"))
	if found || pos != 2 {
		t.Fatalf("search d = %d,%v want 2,false", pos, found)
	}
	pos, found, _ = f.search([]byte("z"))
	if found || pos != 4 {
		t.Fatalf("search z = %d,%v want 4,false", pos, found)
	}
}

func TestSortedFileIterScan(t *testing.T) {
	f := buildSST(t, [][2]string{{"a", "1"}, {"c", "3"}, {"e", "5"}})

	it, _ := f.Iter()
	var fwd []string
	for ; it.Valid(); it.Next() {
		fwd = append(fwd, string(it.Key())+string(it.Val()))
	}
	if len(fwd) != 3 || fwd[0] != "a1" || fwd[2] != "e5" {
		t.Fatalf("forward: %v", fwd)
	}

	it, _ = f.Seek([]byte("b"))
	if !it.Valid() || string(it.Key()) != "c" {
		t.Fatalf("seek b -> %q", it.Key())
	}

	// backward from the end
	it, _ = f.Seek([]byte("\xff"))
	it.Prev()
	var bwd []string
	for ; it.Valid(); it.Prev() {
		bwd = append(bwd, string(it.Key()))
	}
	if len(bwd) != 3 || bwd[0] != "e" || bwd[2] != "a" {
		t.Fatalf("backward: %v", bwd)
	}
}
