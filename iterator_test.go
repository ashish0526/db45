package db

import "testing"

func TestKVIteratorSeekAndScan(t *testing.T) {
	kv, _ := openKV(t)
	for _, k := range []string{"a", "c", "e", "g"} {
		kv.Set([]byte(k), []byte(k+"!"))
	}

	// Seek lands on the first key >= arg
	it, _ := kv.Seek([]byte("d"))
	if !it.Valid() || string(it.Key()) != "e" {
		t.Fatalf("seek d -> %q valid=%v", it.Key(), it.Valid())
	}

	// forward scan to the end
	var fwd []string
	for it, _ := kv.Seek([]byte("")); it.Valid(); it.Next() {
		fwd = append(fwd, string(it.Key()))
	}
	if got := len(fwd); got != 4 || fwd[0] != "a" || fwd[3] != "g" {
		t.Fatalf("forward scan: %v", fwd)
	}

	// backward scan using the "one past the end" trick
	it, _ = kv.Seek([]byte("\xff"))
	it.Prev()
	var bwd []string
	for ; it.Valid(); it.Prev() {
		bwd = append(bwd, string(it.Key()))
	}
	if got := len(bwd); got != 4 || bwd[0] != "g" || bwd[3] != "a" {
		t.Fatalf("backward scan: %v", bwd)
	}
}

func TestKVIteratorPastEnds(t *testing.T) {
	kv, _ := openKV(t)
	kv.Set([]byte("x"), []byte("1"))

	it, _ := kv.Seek([]byte("x"))
	it.Prev() // pos = -1
	if it.Valid() {
		t.Fatal("valid at pos -1")
	}
	it.Next() // back to 0
	if !it.Valid() || string(it.Key()) != "x" {
		t.Fatalf("Next from -1 should return to 0")
	}
	it.Next() // pos = 1 (past end)
	if it.Valid() {
		t.Fatal("valid past end")
	}
}
