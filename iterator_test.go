package db

import "testing"

// These exercise the low-level SortedArray cursor (the "one past each end"
// trick). KV.Seek now layers a merge + tombstone filter on top; see
// merge_test.go and kv_test.go for that path.

func TestSortedArrayIterSeekAndScan(t *testing.T) {
	kv, _ := openKV(t)
	for _, k := range []string{"a", "c", "e", "g"} {
		kv.Set([]byte(k), []byte(k+"!"))
	}

	it, _ := kv.mem.Seek([]byte("d"))
	if !it.Valid() || string(it.Key()) != "e" {
		t.Fatalf("seek d -> %q valid=%v", it.Key(), it.Valid())
	}

	var fwd []string
	for it, _ := kv.mem.Seek([]byte("")); it.Valid(); it.Next() {
		fwd = append(fwd, string(it.Key()))
	}
	if got := len(fwd); got != 4 || fwd[0] != "a" || fwd[3] != "g" {
		t.Fatalf("forward scan: %v", fwd)
	}

	it, _ = kv.mem.Seek([]byte("\xff"))
	it.Prev()
	var bwd []string
	for ; it.Valid(); it.Prev() {
		bwd = append(bwd, string(it.Key()))
	}
	if got := len(bwd); got != 4 || bwd[0] != "g" || bwd[3] != "a" {
		t.Fatalf("backward scan: %v", bwd)
	}
}

func TestSortedArrayIterPastEnds(t *testing.T) {
	kv, _ := openKV(t)
	kv.Set([]byte("x"), []byte("1"))

	it, _ := kv.mem.Seek([]byte("x"))
	it.Prev()
	if it.Valid() {
		t.Fatal("valid at pos -1")
	}
	it.Next()
	if !it.Valid() || string(it.Key()) != "x" {
		t.Fatal("Next from -1 should return to 0")
	}
	it.Next()
	if it.Valid() {
		t.Fatal("valid past end")
	}
}
