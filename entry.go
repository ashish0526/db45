package db

import (
	"encoding/binary"
	"io"
)

// Entry is one logged state change. On disk it is a length-prefixed binary blob:
//
//	| key size | val size | deleted | key data | val data |
//	| 4 bytes  | 4 bytes  | 1 byte  |   ...    |   ...    |
//
// The deleted flag lets a single record type represent both "set" and "del".
// Length-prefixing (rather than delimiters + escaping) is what keeps a storage
// engine's record format simple, fast and able to carry raw binary.
type Entry struct {
	key     []byte
	val     []byte
	deleted bool
}

// Encode returns the record as a freshly allocated byte slice.
func (ent *Entry) Encode() []byte {
	data := make([]byte, 4+4+1+len(ent.key)+len(ent.val))
	binary.LittleEndian.PutUint32(data[0:4], uint32(len(ent.key)))
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(ent.val)))
	if ent.deleted {
		data[8] = 1
	}
	copy(data[9:], ent.key)
	copy(data[9+len(ent.key):], ent.val)
	return data
}

// Decode reads one record from r. It takes an io.Reader, not a []byte, because
// the caller does not know how many bytes a record occupies until it has read
// the length fields. A clean end of stream surfaces as io.EOF; a stream that
// ends partway through a record surfaces as io.ErrUnexpectedEOF.
func (ent *Entry) Decode(r io.Reader) error {
	var hdr [9]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err // io.EOF here means "clean end of the stream"
	}
	klen := binary.LittleEndian.Uint32(hdr[0:4])
	vlen := binary.LittleEndian.Uint32(hdr[4:8])
	ent.deleted = hdr[8] != 0

	body := make([]byte, klen+vlen) // one allocation for key+val
	if _, err := io.ReadFull(r, body); err != nil {
		return err // a short read mid-record => io.ErrUnexpectedEOF
	}
	ent.key = body[:klen:klen] // full-slice bound so appends can't bleed into val
	ent.val = body[klen:]
	return nil
}
