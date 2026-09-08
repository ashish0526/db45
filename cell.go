package db

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

// Cell is a single typed value — the database's only two primitive types. int64
// is the representative fixed-width type (endianness, two's complement, and in
// Chapter 4 the order-preserving key encoding); []byte is the representative
// variable-length type (length-prefixing, and in Chapter 4 escaped key bytes).
// Every other SQL type is a variation on one of these and adds no new
// implementation idea. Go has no union, so the unused field is simply wasted.
type Cell struct {
	Type CellType
	I64  int64
	Str  []byte
}

var errShortCell = errors.New("cell: truncated data")

// Encode appends this cell's serialized value to toAppend and returns the grown
// slice. The type tag is not stored — a row's schema records each column's type,
// so Decode is told the type by its caller. Passing the growing buffer through
// each cell of a row avoids N allocations (a recurring Go append-style idiom).
func (cell *Cell) Encode(toAppend []byte) []byte {
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

// Decode parses one value from the front of data into cell (whose Type the
// caller has already set from the schema) and returns the unconsumed rest.
func (cell *Cell) Decode(data []byte) (rest []byte, err error) {
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
