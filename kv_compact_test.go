package db

import (
	"os"
	"testing"
)

func openKVWithMain(t *testing.T, dir string) *KV { return openKVDir(t, dir) }

func TestKVCompaction(t *testing.T) {
	dir := t.TempDir()
	kv := openKVWithMain(t, dir)

	kv.Set([]byte("a"), []byte("1"))
	kv.Set([]byte("b"), []byte("2"))
	kv.Set([]byte("c"), []byte("3"))
	kv.Del([]byte("b"))

	if err := kv.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// log is now empty; the new SSTable is the sole level and keeps b's tombstone
	if fi, _ := os.Stat(kv.log.FileName); fi.Size() != 0 {
		t.Fatalf("log not truncated: %d bytes", fi.Size())
	}
	if kv.mem.Size() != 0 {
		t.Fatalf("memtable not cleared: %d", kv.mem.Size())
	}
	if len(kv.main) != 1 || kv.main[0].Size() != 3 {
		t.Fatalf("levels=%d, sstable size=%d, want 1 level / 3 records (incl tombstone)", len(kv.main), kv.main[0].Size())
	}

	for k, want := range map[string]string{"a": "1", "c": "3"} {
		v, ok, _ := kv.Get([]byte(k))
		if !ok || string(v) != want {
			t.Fatalf("post-compact Get %q = %q,%v", k, v, ok)
		}
	}
	if _, ok, _ := kv.Get([]byte("b")); ok {
		t.Fatal("deleted key b survived compaction")
	}

	// writes after compaction go to the fresh MemTable and still merge on read
	kv.Set([]byte("b"), []byte("20"))
	kv.Set([]byte("a"), []byte("10")) // shadows the SSTable copy
	if v, _, _ := kv.Get([]byte("a")); string(v) != "10" {
		t.Fatalf("MemTable should shadow SSTable: got %q", v)
	}

	// reopen: SSTable + a fresh (empty) log
	kv.Close()
	kv2 := openKVWithMain(t, dir)
	if v, ok, _ := kv2.Get([]byte("c")); !ok || string(v) != "3" {
		t.Fatalf("after reopen Get c = %q,%v", v, ok)
	}
	// the post-compact MemTable writes were only in the log, which... still exists
	if v, ok, _ := kv2.Get([]byte("b")); !ok || string(v) != "20" {
		t.Fatalf("after reopen Get b = %q,%v (log replay)", v, ok)
	}
}

func TestKVCompactionMetadataCommit(t *testing.T) {
	dir := t.TempDir()
	kv := openKVWithMain(t, dir)

	kv.Set([]byte("a"), []byte("1"))
	kv.Compact() // -> sstable_1
	kv.Set([]byte("b"), []byte("2"))
	kv.Compact() // -> sstable_2 prepended; both files kept as separate levels

	if len(kv.main) != 2 {
		t.Fatalf("expected 2 SSTable levels, got %d", len(kv.main))
	}
	for _, name := range []string{"sstable_1", "sstable_2"} {
		if _, err := os.Stat(dir + "/" + name); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}

	// reopen: the metadata level list alone tells Open which files are live
	kv.Close()
	kv2 := openKVWithMain(t, dir)
	if len(kv2.main) != 2 {
		t.Fatalf("after reopen: %d levels", len(kv2.main))
	}
	for k, want := range map[string]string{"a": "1", "b": "2"} {
		if v, ok, _ := kv2.Get([]byte(k)); !ok || string(v) != want {
			t.Fatalf("after reopen Get %q = %q,%v", k, v, ok)
		}
	}
}

func TestKVManyLevelsShadowing(t *testing.T) {
	dir := t.TempDir()
	kv := openKVWithMain(t, dir)

	kv.Set([]byte("k"), []byte("v1"))
	kv.Set([]byte("old"), []byte("keep"))
	kv.Compact() // level: sstable_1 {k=v1, old=keep}

	kv.Set([]byte("k"), []byte("v2"))
	kv.Compact() // level: sstable_2 {k=v2}  over sstable_1

	kv.Set([]byte("k"), []byte("v3"))
	kv.Del([]byte("old"))
	kv.Compact() // level: sstable_3 {k=v3, old=tombstone}

	if len(kv.main) != 3 {
		t.Fatalf("levels = %d", len(kv.main))
	}
	if v, ok, _ := kv.Get([]byte("k")); !ok || string(v) != "v3" {
		t.Fatalf("newest level should win: got %q ok=%v", v, ok)
	}
	if _, ok, _ := kv.Get([]byte("old")); ok {
		t.Fatal("tombstone in a middle level failed to shadow the oldest")
	}

	it, _ := kv.Seek([]byte(""))
	var got []string
	for ; it.Valid(); it.Next() {
		got = append(got, string(it.Key())+"="+string(it.Val()))
	}
	if len(got) != 1 || got[0] != "k=v3" {
		t.Fatalf("merged scan across 3 levels = %v", got)
	}
}

func TestKVCompactionRangeScan(t *testing.T) {
	dir := t.TempDir()
	kv := openKVWithMain(t, dir)
	for _, k := range []string{"d", "a", "f", "c"} {
		kv.Set([]byte(k), []byte(k))
	}
	kv.Compact()
	kv.Set([]byte("b"), []byte("b")) // lands in the new MemTable
	kv.Del([]byte("d"))              // tombstone in MemTable shadows SSTable

	it, _ := kv.Seek([]byte(""))
	var got []string
	for ; it.Valid(); it.Next() {
		got = append(got, string(it.Key()))
	}
	want := []string{"a", "b", "c", "f"}
	if len(got) != len(want) {
		t.Fatalf("merged scan = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged scan = %v want %v", got, want)
		}
	}
}
