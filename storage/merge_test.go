package storage

import "testing"

func ms(pairs ...[2]string) *memSorted { return newMem(pairs...) }

func drain(it SortedKVIter) []string {
	var out []string
	for ; it.Valid(); it.Next() {
		out = append(out, string(it.Key())+"="+string(it.Val()))
	}
	return out
}

func TestMergeTopLevelWinsNoDuplicates(t *testing.T) {
	top := ms([2]string{"a", "1"}, [2]string{"c", "30"}, [2]string{"z", "99"})
	bot := ms([2]string{"a", "100"}, [2]string{"b", "2"}, [2]string{"c", "3"}, [2]string{"d", "4"})

	it, err := MergedSortedKV{top, bot}.Iter()
	if err != nil {
		t.Fatal(err)
	}
	got := drain(it)
	want := []string{"a=1", "b=2", "c=30", "d=4", "z=99"}
	if len(got) != len(want) {
		t.Fatalf("merged = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged[%d] = %q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestMergeDirectionFlip(t *testing.T) {
	top := ms([2]string{"a", "1"}, [2]string{"c", "3"})
	bot := ms([2]string{"b", "2"}, [2]string{"d", "4"}, [2]string{"e", "5"})

	it, _ := MergedSortedKV{top, bot}.Iter()
	// forward: a, b, c
	for _, want := range []string{"a", "b", "c"} {
		if !it.Valid() || string(it.Key()) != want {
			t.Fatalf("forward: got %q want %q", it.Key(), want)
		}
		it.Next()
	}
	// now at d; flip backward should retrace c, b, a
	if string(it.Key()) != "d" {
		t.Fatalf("expected d, got %q", it.Key())
	}
	for _, want := range []string{"c", "b", "a"} {
		if err := it.Prev(); err != nil {
			t.Fatal(err)
		}
		if !it.Valid() || string(it.Key()) != want {
			t.Fatalf("backward: got %q want %q", it.Key(), want)
		}
	}
	it.Prev()
	if it.Valid() {
		t.Fatalf("expected exhausted, at %q", it.Key())
	}
}

func TestMergeSingleLevel(t *testing.T) {
	it, _ := MergedSortedKV{ms([2]string{"x", "1"}, [2]string{"y", "2"})}.Iter()
	if got := drain(it); len(got) != 2 || got[0] != "x=1" {
		t.Fatalf("single level merge: %v", got)
	}
}
