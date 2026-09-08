package db

import "errors"

// Column is one named, typed column of a table.
type Column struct {
	Name string
	Type CellType
}

// Schema describes a table: its name, its columns, and which columns (by index
// into Cols) form the primary key.
type Schema struct {
	Table string
	Cols  []Column
	PKey  []int
}

// colIndex returns the index of the named column, or -1.
func (schema *Schema) colIndex(name string) int {
	for i := range schema.Cols {
		if schema.Cols[i].Name == name {
			return i
		}
	}
	return -1
}

// isPKey reports whether column index i is part of the primary key.
func (schema *Schema) isPKey(i int) bool {
	for _, p := range schema.PKey {
		if p == i {
			return true
		}
	}
	return false
}

// Row is one row: a Cell per column, positionally matching Schema.Cols.
type Row []Cell

// NewRow returns a zeroed row shaped for this schema.
func (schema *Schema) NewRow() Row { return make(Row, len(schema.Cols)) }

// EncodeKey builds the KV key: the table prefix ("name\x00", whose 0x00
// separator stops tables "ab" and "abc" from colliding), then each primary-key
// column preceded by its one-byte type tag (1..2, never 0xff), then a trailing
// 0x00. The type tag guarantees a real column never starts with 0xff, so 0xff is
// free to mean "+infinity" in a prefix bound; the trailing 0x00 (one byte > "")
// keeps a full key sorting after the same prefix padded with -infinity.
func (row Row) EncodeKey(schema *Schema) (key []byte) {
	key = append([]byte(schema.Table), 0x00)
	for _, idx := range schema.PKey {
		key = append(key, byte(row[idx].Type))
		key = row[idx].EncodeKey(key)
	}
	return append(key, 0x00)
}

// EncodeKeyPrefix builds a partial key from the first prefix columns of a
// primary key, padded with a sentinel for the open end: 0xff (+infinity) when
// positive, nothing (-infinity) otherwise. Turning "(a,b) OP (x,y)" into a
// single byte-string comparison is the crux of every real range scan.
func EncodeKeyPrefix(schema *Schema, prefix []Cell, positive bool) []byte {
	key := append([]byte(schema.Table), 0x00)
	for i := range prefix {
		key = append(key, byte(prefix[i].Type))
		key = prefix[i].EncodeKey(key)
	}
	if positive {
		key = append(key, 0xff)
	}
	return key
}

// EncodeVal builds the KV value: every non-primary-key column, in column order.
func (row Row) EncodeVal(schema *Schema) (val []byte) {
	for i := range schema.Cols {
		if !schema.isPKey(i) {
			val = row[i].EncodeVal(val)
		}
	}
	return val
}

// ErrOutOfRange means a decoded KV key belongs to a different table — the signal
// that a table scan has run past the end of its keyspace.
var ErrOutOfRange = errors.New("row: key does not belong to this table")

// DecodeKey fills the primary-key cells of row from a KV key.
func (row Row) DecodeKey(schema *Schema, key []byte) error {
	prefix := schema.Table + "\x00"
	if len(key) < len(prefix) || string(key[:len(prefix)]) != prefix {
		return ErrOutOfRange
	}
	rest := key[len(prefix):]
	for _, idx := range schema.PKey {
		if len(rest) < 1 {
			return errShortCell
		}
		rest = rest[1:] // skip the one-byte type tag
		row[idx].Type = schema.Cols[idx].Type
		var err error
		if rest, err = row[idx].DecodeKey(rest); err != nil {
			return err
		}
	}
	return nil // a trailing 0x00 may remain; it is not part of any column
}

// DecodeVal fills the non-primary-key cells of row from a KV value.
func (row Row) DecodeVal(schema *Schema, val []byte) error {
	rest := val
	for i := range schema.Cols {
		if schema.isPKey(i) {
			continue
		}
		row[i].Type = schema.Cols[i].Type
		var err error
		if rest, err = row[i].DecodeVal(rest); err != nil {
			return err
		}
	}
	return nil
}
