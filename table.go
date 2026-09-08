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
// separator stops tables "ab" and "abc" from colliding) followed by each primary
// key column. The primary key IS the KV key — that is how a plain KV store
// becomes an OLTP relational database.
func (row Row) EncodeKey(schema *Schema) (key []byte) {
	key = append([]byte(schema.Table), 0x00)
	for _, idx := range schema.PKey {
		key = row[idx].EncodeKey(key)
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
		row[idx].Type = schema.Cols[idx].Type
		var err error
		if rest, err = row[idx].DecodeKey(rest); err != nil {
			return err
		}
	}
	return nil
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
