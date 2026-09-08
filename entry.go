package db

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

// ErrBadSum means a record's stored checksum did not match its bytes — a torn
// write or corruption. On the last record of a log it is treated as a clean end.
var ErrBadSum = errors.New("bad checksum")

// Entry is one logged state change. On disk it is a length-prefixed binary blob
// with a leading CRC over everything that follows it:
//
//	|  crc32  | key size | val size | deleted | key data | val data |
//	| 4 bytes | 4 bytes  | 4 bytes  | 1 byte  |   ...    |   ...    |
//
// The CRC turns "corrupt" into "detectably corrupt": recompute on read, and a
// mismatch means the record was not fully written, so replay stops there.
type Entry struct {
	key     []byte
	val     []byte
	deleted bool
}

const entryHeaderLen = 4 + 4 + 1 // key size, val size, deleted (the crc-covered header)

// Encode returns the record as a freshly allocated byte slice.
func (ent *Entry) Encode() []byte {
	data := make([]byte, 4+entryHeaderLen+len(ent.key)+len(ent.val))
	rest := data[4:]
	binary.LittleEndian.PutUint32(rest[0:4], uint32(len(ent.key)))
	binary.LittleEndian.PutUint32(rest[4:8], uint32(len(ent.val)))
	if ent.deleted {
		rest[8] = 1
	}
	copy(rest[entryHeaderLen:], ent.key)
	copy(rest[entryHeaderLen+len(ent.key):], ent.val)
	binary.LittleEndian.PutUint32(data[0:4], crc32.ChecksumIEEE(rest))
	return data
}

// Decode reads one record from r and returns exactly one of:
//
//	nil                  record OK
//	io.EOF               clean end of stream (all records valid)
//	io.ErrUnexpectedEOF  stream ended mid-record (torn write)
//	ErrBadSum            checksum mismatch (torn write / corruption)
func (ent *Entry) Decode(r io.Reader) error {
	var hdr [4 + entryHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err // io.EOF (clean) or io.ErrUnexpectedEOF (torn header)
	}
	want := binary.LittleEndian.Uint32(hdr[0:4])
	klen := binary.LittleEndian.Uint32(hdr[4:8])
	vlen := binary.LittleEndian.Uint32(hdr[8:12])
	ent.deleted = hdr[12] != 0

	body := make([]byte, klen+vlen)
	if _, err := io.ReadFull(r, body); err != nil {
		if err == io.EOF {
			return io.ErrUnexpectedEOF // header promised a body that isn't there
		}
		return err
	}

	sum := crc32.NewIEEE()
	sum.Write(hdr[4:]) // the crc-covered header
	sum.Write(body)
	if sum.Sum32() != want {
		return ErrBadSum
	}

	ent.key = body[:klen:klen]
	ent.val = body[klen:]
	return nil
}
