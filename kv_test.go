package db

import (
	"bytes"
	"path/filepath"
	"testing"
)

// openKV returns an opened KV backed by a fresh log file in a temp dir, plus the
// log path so a test can re-open it to simulate a restart.
func openKV(t *testing.T) (*KV, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kv_log")
	kv := &KV{}
	kv.log.FileName = path
	if err := kv.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { kv.Close() })
	return kv, path
}

func TestKVGetSetDel(t *testing.T) {
	kv, _ := openKV(t)

	if _, ok, err := kv.Get([]byte("k1")); err != nil || ok {
		t.Fatalf("Get missing: ok=%v err=%v", ok, err)
	}

	if updated, err := kv.Set([]byte("k1"), []byte("x")); err != nil || updated {
		t.Fatalf("Set new: updated=%v err=%v", updated, err)
	}
	if v, ok, err := kv.Get([]byte("k1")); err != nil || !ok || !bytes.Equal(v, []byte("x")) {
		t.Fatalf("Get k1: v=%q ok=%v err=%v", v, ok, err)
	}

	if updated, err := kv.Set([]byte("k1"), []byte("y")); err != nil || !updated {
		t.Fatalf("Set existing: updated=%v err=%v", updated, err)
	}
	if v, _, _ := kv.Get([]byte("k1")); !bytes.Equal(v, []byte("y")) {
		t.Fatalf("Get k1 after update: v=%q", v)
	}

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

func TestKVDurability(t *testing.T) {
	kv, path := openKV(t)

	kv.Set([]byte("a"), []byte("1"))
	kv.Set([]byte("b"), []byte("2"))
	kv.Set([]byte("a"), []byte("3")) // overwrite
	kv.Del([]byte("b"))
	kv.Set([]byte("c"), []byte("4"))
	kv.Close()

	// "restart": a fresh KV over the same log must see the replayed state.
	kv2 := &KV{}
	kv2.log.FileName = path
	if err := kv2.Open(); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer kv2.Close()

	want := map[string]string{"a": "3", "c": "4"}
	for k, w := range want {
		v, ok, _ := kv2.Get([]byte(k))
		if !ok || string(v) != w {
			t.Fatalf("after replay Get %q: v=%q ok=%v want %q", k, v, ok, w)
		}
	}
	if _, ok, _ := kv2.Get([]byte("b")); ok {
		t.Fatalf("deleted key b came back after replay")
	}
}
