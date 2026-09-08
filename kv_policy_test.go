package db

import (
	"os"
	"strconv"
	"testing"
)

func TestCompactionPolicyMergesAndCascades(t *testing.T) {
	dir := t.TempDir()
	kv := &KV{Options: KVOptions{Dirpath: dir, LogShreshold: 2, GrowthFactor: 2}}
	if err := kv.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { kv.Close() })

	// Each Compact flushes ~2 keys into a new top level; the growth policy then
	// folds oversized levels downward.
	for round := 0; round < 6; round++ {
		kv.Set([]byte("k"+strconv.Itoa(2*round)), []byte("v"))
		kv.Set([]byte("k"+strconv.Itoa(2*round+1)), []byte("v"))
		if err := kv.Compact(); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}

	// With a growth factor of 2 the level count stays logarithmic, not linear.
	if len(kv.main) > 4 {
		t.Fatalf("policy failed to bound levels: %d for 12 keys", len(kv.main))
	}

	// all keys still readable
	for i := 0; i < 12; i++ {
		if _, ok, _ := kv.Get([]byte("k" + strconv.Itoa(i))); !ok {
			t.Fatalf("k%d lost after compaction", i)
		}
	}

	// superseded files are deleted: only the live level files remain
	live := map[string]bool{}
	for _, n := range kv.meta.Get().SSTables {
		live[n] = true
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		n := e.Name()
		if len(n) > 8 && n[:8] == "sstable_" && !live[n] {
			t.Fatalf("orphan SSTable left on disk: %s", n)
		}
	}
}

func TestCompactionPolicyDropsTombstonesAtLastLevel(t *testing.T) {
	dir := t.TempDir()
	kv := &KV{Options: KVOptions{Dirpath: dir, LogShreshold: 1, GrowthFactor: 2}}
	kv.Open()
	t.Cleanup(func() { kv.Close() })

	kv.Set([]byte("a"), []byte("1"))
	kv.Set([]byte("b"), []byte("2"))
	kv.Compact()
	kv.Del([]byte("a"))
	kv.Set([]byte("c"), []byte("3"))
	kv.Compact()
	kv.Set([]byte("d"), []byte("4"))
	kv.Compact()

	// force everything down to a single level
	for i := 0; i < 5; i++ {
		kv.Set([]byte("z"+strconv.Itoa(i)), []byte("z"))
		kv.Compact()
	}

	if _, ok, _ := kv.Get([]byte("a")); ok {
		t.Fatal("deleted key survived merges")
	}
	// once merged into the final level, a's tombstone should be physically gone
	last := &kv.main[len(kv.main)-1]
	it, _ := last.Iter()
	for ; it.Valid(); it.Next() {
		if string(it.Key()) == "a" {
			t.Fatal("tombstone for 'a' still present in the last level")
		}
	}
}
