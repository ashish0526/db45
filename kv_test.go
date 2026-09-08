package db

import (
	"bytes"
	"testing"
)

func TestKVGetSetDel(t *testing.T) {
	var kv KV
	if err := kv.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer kv.Close()

	// missing key
	if _, ok, err := kv.Get([]byte("k1")); err != nil || ok {
		t.Fatalf("Get missing: val ok=%v err=%v", ok, err)
	}

	// first Set is an insert
	if updated, err := kv.Set([]byte("k1"), []byte("x")); err != nil || updated {
		t.Fatalf("Set new: updated=%v err=%v", updated, err)
	}
	if v, ok, err := kv.Get([]byte("k1")); err != nil || !ok || !bytes.Equal(v, []byte("x")) {
		t.Fatalf("Get k1: v=%q ok=%v err=%v", v, ok, err)
	}

	// second Set is an update
	if updated, err := kv.Set([]byte("k1"), []byte("y")); err != nil || !updated {
		t.Fatalf("Set existing: updated=%v err=%v", updated, err)
	}
	if v, _, _ := kv.Get([]byte("k1")); !bytes.Equal(v, []byte("y")) {
		t.Fatalf("Get k1 after update: v=%q", v)
	}

	// delete reports whether something was removed
	if deleted, err := kv.Del([]byte("k1")); err != nil || !deleted {
		t.Fatalf("Del present: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := kv.Del([]byte("k1")); err != nil || deleted {
		t.Fatalf("Del absent: deleted=%v err=%v", deleted, err)
	}
	if _, ok, _ := kv.Get([]byte("k1")); ok {
		t.Fatalf("k1 still present after Del")
	}
}
