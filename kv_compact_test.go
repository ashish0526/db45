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

	// log is now empty; SSTable holds the live data, tombstone physically gone
	if fi, _ := os.Stat(kv.log.FileName); fi.Size() != 0 {
		t.Fatalf("log not truncated: %d bytes", fi.Size())
	}
	if kv.mem.Size() != 0 {
		t.Fatalf("memtable not cleared: %d", kv.mem.Size())
	}
	if kv.main.Size() != 2 {
		t.Fatalf("sstable has %d keys, want 2 (b's tombstone dropped)", kv.main.Size())
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
	kv.Compact() // -> sstable_2, sstable_1 deleted

	if _, err := os.Stat(dir + "/sstable_1"); !os.IsNotExist(err) {
		t.Fatalf("superseded sstable_1 not deleted: %v", err)
	}
	if _, err := os.Stat(dir + "/sstable_2"); err != nil {
		t.Fatalf("sstable_2 missing: %v", err)
	}

	// reopen: the metadata pointer alone tells Open which file is live
	kv.Close()
	kv2 := openKVWithMain(t, dir)
	for k, want := range map[string]string{"a": "1", "b": "2"} {
		if v, ok, _ := kv2.Get([]byte(k)); !ok || string(v) != want {
			t.Fatalf("after reopen Get %q = %q,%v", k, v, ok)
		}
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
