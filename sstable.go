package db

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

// SortedKV is any source of sorted key/value pairs the SSTable builder and the
// merge layer consume: the in-memory array, a merged view of several levels, or
// a test mock. EstimatedSize may exceed the true count (a merge drops duplicates
// and tombstones); the builder handles the slack.
type SortedKV interface {
	EstimatedSize() int
	Iter() (SortedKVIter, error)
	Seek(key []byte) (SortedKVIter, error)
}

// SortedKVIter is a cursor over sorted KV data. Deleted reports whether the
// current entry is a tombstone (a delete marker that shadows lower levels during
// a merge and is filtered out of query results).
type SortedKVIter interface {
	Valid() bool
	Key() []byte
	Val() []byte
	Deleted() bool
	Next() error
	Prev() error
}

// SortedFile is an immutable sorted KV file on disk:
//
//	[ n keys | offset[0] | offset[1] | ... | offset[n-1] | KV[0] | KV[1] | ... ]
//	   8B         8B          8B                             (each KV:)
//	                              [ key length | val length | key data | val data ]
//	                                   4B           4B
//
// The offset array IS the search index: binary search reads offset[i] by index
// with ReadAt, then reads that KV — no need to load the file into memory.
type SortedFile struct {
	FileName string
	fp       *os.File
	nkeys    int
}

// Open opens an existing SSTable and reads its key count.
func (f *SortedFile) Open() error {
	fp, err := os.OpenFile(f.FileName, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	f.fp = fp
	var buf [8]byte
	if _, err := fp.ReadAt(buf[:], 0); err != nil {
		return err
	}
	f.nkeys = int(binary.LittleEndian.Uint64(buf[:]))
	return nil
}

// Close closes the underlying file.
func (f *SortedFile) Close() error {
	if f.fp == nil {
		return nil
	}
	return f.fp.Close()
}

// Size reports the number of records.
func (f *SortedFile) Size() int { return f.nkeys }

// EstimatedSize satisfies SortedKV. For a written file it is exact.
func (f *SortedFile) EstimatedSize() int { return f.nkeys }

// index reads the pos-th record (0-based) via positioned I/O.
func (f *SortedFile) index(pos int) (key, val []byte, err error) {
	if pos < 0 || pos >= f.nkeys {
		return nil, nil, errors.New("sstable: index out of range")
	}
	var buf [8]byte
	if _, err = f.fp.ReadAt(buf[:], int64(8+8*pos)); err != nil {
		return nil, nil, err
	}
	offset := int64(binary.LittleEndian.Uint64(buf[:]))
	if offset < int64(8+8*f.nkeys) {
		return nil, nil, errors.New("sstable: corrupted file")
	}
	if _, err = f.fp.ReadAt(buf[:], offset); err != nil {
		return nil, nil, err
	}
	klen := binary.LittleEndian.Uint32(buf[0:4])
	vlen := binary.LittleEndian.Uint32(buf[4:8])
	body := make([]byte, klen+vlen)
	if _, err = f.fp.ReadAt(body, offset+8); err != nil {
		return nil, nil, err
	}
	return body[:klen:klen], body[klen:], nil
}

// search returns the position of the first key >= target and whether it is an
// exact match. Binary search directly on the on-disk offset array.
func (f *SortedFile) search(target []byte) (int, bool, error) {
	lo, hi := 0, f.nkeys
	for lo < hi {
		mid := (lo + hi) / 2
		k, _, err := f.index(mid)
		if err != nil {
			return 0, false, err
		}
		switch bytes.Compare(k, target) {
		case 0:
			return mid, true, nil
		case -1:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return lo, false, nil
}

// SortedFileIter walks an SSTable. key/val are cached eagerly after each move so
// Key()/Val() are getters and I/O errors surface through Next/Prev.
type SortedFileIter struct {
	file *SortedFile
	pos  int
	key  []byte
	val  []byte
}

func (it *SortedFileIter) Valid() bool   { return 0 <= it.pos && it.pos < it.file.nkeys }
func (it *SortedFileIter) Key() []byte   { return it.key }
func (it *SortedFileIter) Val() []byte   { return it.val }
func (it *SortedFileIter) Deleted() bool { return false } // SSTables at this stage hold no tombstones

func (it *SortedFileIter) load() error {
	if !it.Valid() {
		it.key, it.val = nil, nil
		return nil
	}
	k, v, err := it.file.index(it.pos)
	if err != nil {
		return err
	}
	it.key, it.val = k, v
	return nil
}

func (it *SortedFileIter) Next() error {
	if it.pos < it.file.nkeys {
		it.pos++
	}
	return it.load()
}

func (it *SortedFileIter) Prev() error {
	if it.pos >= 0 {
		it.pos--
	}
	return it.load()
}

// Iter returns a cursor at the start of the file.
func (f *SortedFile) Iter() (SortedKVIter, error) {
	it := &SortedFileIter{file: f, pos: 0}
	return it, it.load()
}

// Seek returns a cursor positioned at the first key >= key.
func (f *SortedFile) Seek(key []byte) (SortedKVIter, error) {
	pos, _, err := f.search(key)
	if err != nil {
		return nil, err
	}
	it := &SortedFileIter{file: f, pos: pos}
	return it, it.load()
}

// CreateFromSorted writes kv's contents to f in one pass. The offset array is
// sized from EstimatedSize (an upper bound); records stream into the data region
// while offset slots are filled out of order with WriteAt. After iterating, the
// true count is written back to the header — the unused offset slots become a
// harmless gap:
//
//	[ n (true) | offset[0..est-1] | gap | KV records ]
func (f *SortedFile) CreateFromSorted(kv SortedKV) error {
	fp, err := createFileSync(f.FileName)
	if err != nil {
		return err
	}
	f.fp = fp

	est := kv.EstimatedSize()
	dataStart := int64(8 + 8*est)
	pos := dataStart
	off := make([]byte, 8)

	it, err := kv.Iter()
	if err != nil {
		return err
	}
	n := 0
	for ; it.Valid(); n++ {
		rec := encodeKV(it.Key(), it.Val())
		if err := writeAllAt(fp, pos, rec); err != nil {
			return err
		}
		binary.LittleEndian.PutUint64(off, uint64(pos))
		if err := writeAllAt(fp, int64(8+8*n), off); err != nil {
			return err
		}
		pos += int64(len(rec))
		if err := it.Next(); err != nil {
			return err
		}
	}
	f.nkeys = n
	if err := writeAllAt(fp, 0, binary.LittleEndian.AppendUint64(nil, uint64(n))); err != nil {
		return err
	}
	return fp.Sync()
}

// encodeKV serializes one KV record: | keylen(4) | vallen(4) | key | val |.
func encodeKV(key, val []byte) []byte {
	rec := make([]byte, 8+len(key)+len(val))
	binary.LittleEndian.PutUint32(rec[0:4], uint32(len(key)))
	binary.LittleEndian.PutUint32(rec[4:8], uint32(len(val)))
	copy(rec[8:], key)
	copy(rec[8+len(key):], val)
	return rec
}

func writeAllAt(fp *os.File, off int64, b []byte) error {
	for len(b) > 0 {
		n, err := fp.WriteAt(b, off)
		if err != nil {
			return err
		}
		b = b[n:]
		off += int64(n)
	}
	return nil
}
