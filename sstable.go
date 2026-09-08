package db

import (
	"encoding/binary"
	"os"
)

// SortedKV is any source of sorted key/value pairs the SSTable builder can
// consume: the in-memory array, a merged view of several levels, or a test mock.
// The builder never knows which.
type SortedKV interface {
	Size() int
	Iter() (SortedKVIter, error)
}

// SortedKVIter is a cursor over sorted KV data (same shape as KVIterator).
type SortedKVIter interface {
	Valid() bool
	Key() []byte
	Val() []byte
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
}

// Close closes the underlying file.
func (f *SortedFile) Close() error {
	if f.fp == nil {
		return nil
	}
	return f.fp.Close()
}

// CreateFromSorted writes kv's contents to f in one pass. Because the source
// reports Size(), n and therefore the byte position where KV data begins are
// known up front, so KVs stream into the data region while the offset slots are
// filled out of order with WriteAt.
func (f *SortedFile) CreateFromSorted(kv SortedKV) error {
	fp, err := createFileSync(f.FileName)
	if err != nil {
		return err
	}
	f.fp = fp

	n := kv.Size()
	if err := writeAllAt(fp, 0, binary.LittleEndian.AppendUint64(nil, uint64(n))); err != nil {
		return err
	}

	dataStart := int64(8 + 8*n)
	pos := dataStart
	off := make([]byte, 8)

	it, err := kv.Iter()
	if err != nil {
		return err
	}
	for i := 0; it.Valid(); i++ {
		rec := encodeKV(it.Key(), it.Val())
		if err := writeAllAt(fp, pos, rec); err != nil {
			return err
		}
		binary.LittleEndian.PutUint64(off, uint64(pos))
		if err := writeAllAt(fp, int64(8+8*i), off); err != nil {
			return err
		}
		pos += int64(len(rec))
		if err := it.Next(); err != nil {
			return err
		}
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
