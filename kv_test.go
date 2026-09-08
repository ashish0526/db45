package db

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
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

	// Set is an upsert: it always writes, so the returned bool ("did a write
	// happen?") is true in both cases. INSERT / UPDATE gating arrives in SetEx.
	if wrote, err := kv.Set([]byte("k1"), []byte("x")); err != nil || !wrote {
		t.Fatalf("Set new: wrote=%v err=%v", wrote, err)
	}
	if v, ok, err := kv.Get([]byte("k1")); err != nil || !ok || !bytes.Equal(v, []byte("x")) {
		t.Fatalf("Get k1: v=%q ok=%v err=%v", v, ok, err)
	}

	if wrote, err := kv.Set([]byte("k1"), []byte("y")); err != nil || !wrote {
		t.Fatalf("Set existing: wrote=%v err=%v", wrote, err)
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

func TestKVSetExModes(t *testing.T) {
	kv, _ := openKV(t)

	// ModeInsert: writes only when absent.
	if wrote, _ := kv.SetEx([]byte("k"), []byte("1"), ModeInsert); !wrote {
		t.Fatal("insert new: want wrote=true")
	}
	if wrote, _ := kv.SetEx([]byte("k"), []byte("2"), ModeInsert); wrote {
		t.Fatal("insert existing: want wrote=false")
	}
	if v, _, _ := kv.Get([]byte("k")); string(v) != "1" {
		t.Fatalf("value clobbered by failed insert: %q", v)
	}

	// ModeUpdate: writes only when present.
	if wrote, _ := kv.SetEx([]byte("k"), []byte("3"), ModeUpdate); !wrote {
		t.Fatal("update existing: want wrote=true")
	}
	if wrote, _ := kv.SetEx([]byte("absent"), []byte("x"), ModeUpdate); wrote {
		t.Fatal("update absent: want wrote=false")
	}
	if _, ok, _ := kv.Get([]byte("absent")); ok {
		t.Fatal("failed update created a key")
	}

	// ModeUpsert always writes.
	if wrote, _ := kv.SetEx([]byte("k"), []byte("4"), ModeUpsert); !wrote {
		t.Fatal("upsert: want wrote=true")
	}
	if v, _, _ := kv.Get([]byte("k")); string(v) != "4" {
		t.Fatalf("upsert value: %q", v)
	}
}

func TestKVKeysStaySorted(t *testing.T) {
	kv, _ := openKV(t)
	ins := []string{"m", "a", "z", "c", "b", "q", "a"} // note the duplicate
	for _, k := range ins {
		kv.Set([]byte(k), []byte("v"))
	}
	kv.Del([]byte("q")) // becomes a tombstone in the MemTable

	if !slices.IsSortedFunc(kv.mem.keys, bytes.Compare) {
		t.Fatalf("keys not sorted: %q", kv.mem.keys)
	}
	// The tombstone-filtered scan yields the live keys in order.
	it, _ := kv.Seek([]byte(""))
	var got []string
	for ; it.Valid(); it.Next() {
		got = append(got, string(it.Key()))
	}
	want := []string{"a", "b", "c", "m", "z"}
	if len(got) != len(want) {
		t.Fatalf("live keys = %q want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("live[%d]=%q want %q", i, got[i], w)
		}
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

// TestKVTornFinalAppend simulates a power cut in the middle of the last append:
// the recovered state must be everything up to the last complete record.
func TestKVTornFinalAppend(t *testing.T) {
	kv, path := openKV(t)
	kv.Set([]byte("a"), []byte("1"))
	kv.Set([]byte("b"), []byte("2"))
	kv.Close()

	// Append a half-written record: a plausible-looking prefix that ends early.
	good := (&Entry{key: []byte("c"), val: []byte("3")}).Encode()
	fp, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fp.Write(good[:len(good)-2])
	fp.Close()

	kv2 := &KV{}
	kv2.log.FileName = path
	if err := kv2.Open(); err != nil {
		t.Fatalf("Open after torn append: %v", err)
	}
	defer kv2.Close()

	if v, ok, _ := kv2.Get([]byte("a")); !ok || string(v) != "1" {
		t.Fatalf("a: v=%q ok=%v", v, ok)
	}
	if v, ok, _ := kv2.Get([]byte("b")); !ok || string(v) != "2" {
		t.Fatalf("b: v=%q ok=%v", v, ok)
	}
	if _, ok, _ := kv2.Get([]byte("c")); ok {
		t.Fatalf("torn record for c was applied")
	}
}
