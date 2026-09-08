package db

import (
	"os"
	"path/filepath"
	"testing"
)

func newMetaStore(t *testing.T, dir string) *KVMetaStore {
	t.Helper()
	m := &KVMetaStore{}
	m.slots[0].FileName = filepath.Join(dir, "meta0")
	m.slots[1].FileName = filepath.Join(dir, "meta1")
	if err := m.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func TestMetaStoreRoundTripAndAlternation(t *testing.T) {
	dir := t.TempDir()
	m := newMetaStore(t, dir)

	if got := m.Get(); got.Version != 0 || len(got.SSTables) != 0 {
		t.Fatalf("fresh Get = %+v", got)
	}

	for v := uint64(1); v <= 5; v++ {
		if err := m.Set(KVMetaData{Version: v, SSTables: []string{"sstable_" + string(rune('0'+v))}}); err != nil {
			t.Fatalf("Set v%d: %v", v, err)
		}
		if got := m.Get(); got.Version != v {
			t.Fatalf("after Set v%d, Get = %+v", v, got)
		}
	}

	// a stale version is rejected
	if err := m.Set(KVMetaData{Version: 3}); err == nil {
		t.Fatal("stale Set should be rejected")
	}

	// reopen: the last write survives
	m.Close()
	m2 := newMetaStore(t, dir)
	if got := m2.Get(); got.Version != 5 {
		t.Fatalf("after reopen Get = %+v", got)
	}
}

func TestMetaStoreSurvivesCorruptSlot(t *testing.T) {
	dir := t.TempDir()
	m := newMetaStore(t, dir)
	m.Set(KVMetaData{Version: 1, SSTables: []string{"a"}})
	m.Set(KVMetaData{Version: 2, SSTables: []string{"b"}}) // slot 1
	m.Close()

	// Corrupt the newer slot (slot 1). Recovery must fall back to slot 0 (v1).
	f, _ := os.OpenFile(filepath.Join(dir, "meta1"), os.O_WRONLY, 0)
	f.WriteAt([]byte{0xde, 0xad, 0xbe, 0xef}, 4)
	f.Close()

	m2 := newMetaStore(t, dir)
	got := m2.Get()
	if got.Version != 1 || len(got.SSTables) != 1 || got.SSTables[0] != "a" {
		t.Fatalf("recovery = %+v want {1 a}", got)
	}

	// and the next Set can still make progress (writing the corrupt slot)
	if err := m2.Set(KVMetaData{Version: 3, SSTables: []string{"c"}}); err != nil {
		t.Fatalf("Set after recovery: %v", err)
	}
	if m2.Get().Version != 3 {
		t.Fatalf("after recovery Set: %+v", m2.Get())
	}
}
