package table

import (
	"encoding/binary"
	"errors"
)

// CellType tags the dynamic type of a Cell.
type CellType uint8

const (
	TypeI64 CellType = 1 // fixed-width: 8 little-endian bytes
	TypeStr CellType = 2 // variable-length: 4-byte length prefix + bytes
)

// Cell is a single typed value — the database's only two primitive types.
type Cell struct {
	Type CellType
	I64  int64
	Str  []byte
}

var errShortCell = errors.New("cell: truncated data")

// --- value encoding (used for KV values; little-endian, compact) ----------------

// EncodeVal appends this cell's serialized value to toAppend. The type tag is not
// stored — a row's schema records each column's type. Passing the growing buffer
// through each cell of a row avoids N allocations.
func (cell *Cell) EncodeVal(toAppend []byte) []byte {
	switch cell.Type {
	case TypeI64:
		// Signed/unsigned integers are the same bits; the cast is a no-op.
		return binary.LittleEndian.AppendUint64(toAppend, uint64(cell.I64))
	case TypeStr:
		out := binary.LittleEndian.AppendUint32(toAppend, uint32(len(cell.Str)))
		return append(out, cell.Str...)
	default:
		panic("cell: unknown type")
	}
}

// DecodeVal parses one value from the front of data into cell (whose Type the
// caller has set from the schema) and returns the unconsumed rest.
func (cell *Cell) DecodeVal(data []byte) (rest []byte, err error) {
	switch cell.Type {
	case TypeI64:
		if len(data) < 8 {
			return nil, errShortCell
		}
		cell.I64 = int64(binary.LittleEndian.Uint64(data))
		return data[8:], nil
	case TypeStr:
		if len(data) < 4 {
			return nil, errShortCell
		}
		n := binary.LittleEndian.Uint32(data)
		data = data[4:]
		if uint32(len(data)) < n {
			return nil, errShortCell
		}
		cell.Str = data[:n:n]
		return data[n:], nil
	default:
		return nil, errors.New("cell: unknown type")
	}
}

// --- order-preserving key encoding --------------------------------------------
//
// Goal: bytes.Compare(EncodeKey(a), EncodeKey(b)) equals the logical comparison
// of a and b, so KV never has to decode keys to sort them. Each column encoding
// is prefix-free (fixed 8 bytes for int64; 0x00-terminated for strings), so
// concatenating columns gives tuple comparison for free.

// EncodeKey appends this cell's order-preserving form to out.
func (cell *Cell) EncodeKey(out []byte) []byte {
	switch cell.Type {
	case TypeI64:
		// 1. big-endian: byte 0 is most-significant, so lexicographic byte order
		//    matches magnitude order (for non-negative values).
		// 2. flip the sign bit: in two's complement it is 1 for negatives, so
		//    unflipped they would sort after positives. Flipping maps the signed
		//    range [-2^63, 2^63) onto the unsigned range [0, 2^64) in order.
		return binary.BigEndian.AppendUint64(out, uint64(cell.I64)^(1<<63))
	case TypeStr:
		return encodeStrKey(out, cell.Str)
	default:
		panic("cell: unknown type")
	}
}

// DecodeKey parses one order-preserving column from the front of data.
func (cell *Cell) DecodeKey(data []byte) (rest []byte, err error) {
	switch cell.Type {
	case TypeI64:
		if len(data) < 8 {
			return nil, errShortCell
		}
		cell.I64 = int64(binary.BigEndian.Uint64(data) ^ (1 << 63))
		return data[8:], nil
	case TypeStr:
		return decodeStrKey(cell, data)
	default:
		return nil, errors.New("cell: unknown type")
	}
}

// A string key column is variable length and followed by more columns, so it
// needs an unambiguous terminator. Use 0x00, escaping any real 0x00/0x01:
//
//	0x00 <=> 0x01 0x01
//	0x01 <=> 0x01 0x02
//
// The escape sequences sort after a bare 0x00 terminator and before 0x02, so a
// prefix still sorts before the longer string ("ab" < "abc").
func encodeStrKey(out, input []byte) []byte {
	for _, ch := range input {
		if ch == 0x00 || ch == 0x01 {
			out = append(out, 0x01, ch+1)
		} else {
			out = append(out, ch)
		}
	}
	return append(out, 0x00) // terminator
}

func decodeStrKey(cell *Cell, data []byte) (rest []byte, err error) {
	var s []byte
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case 0x00:
			cell.Str = s
			return data[i+1:], nil
		case 0x01:
			if i+1 >= len(data) || (data[i+1] != 0x01 && data[i+1] != 0x02) {
				return nil, errors.New("cell: bad key escape")
			}
			s = append(s, data[i+1]-1)
			i++
		default:
			s = append(s, data[i])
		}
	}
	return nil, errShortCell // no terminator
}
