package table

import "github.com/ashish0526/db45/storage"

// RowIterator wraps a storage.SortedKVIter so it yields decoded Rows and stops at
// the table boundary. Both the plain MemTable cursor and a bounded RangedKVIter
// satisfy storage.SortedKVIter, so the row layer does not care which is
// underneath. After each move the row is decoded eagerly — that is how the
// iterator knows whether it is still Valid() (in-table) — and cached, so Row()
// and Valid() are plain getters.
type RowIterator struct {
	schema *Schema
	iter   storage.SortedKVIter
	valid  bool
	row    Row
}

// NewRowIterator wraps a storage cursor as a RowIterator over schema and decodes
// the first position. The sql package uses this to turn a byte-key range scan
// into a stream of rows.
func NewRowIterator(schema *Schema, cur storage.SortedKVIter) (*RowIterator, error) {
	it := &RowIterator{schema: schema, iter: cur}
	if err := it.decode(); err != nil {
		return nil, err
	}
	return it, nil
}

func (it *RowIterator) Valid() bool { return it.valid }
func (it *RowIterator) Row() Row    { return it.row }

func (it *RowIterator) Next() error {
	if err := it.iter.Next(); err != nil {
		return err
	}
	return it.decode()
}

func (it *RowIterator) Prev() error {
	if err := it.iter.Prev(); err != nil {
		return err
	}
	return it.decode()
}

// decode reads the row at the current KV position into it.row and sets it.valid.
func (it *RowIterator) decode() error {
	it.valid = false
	if !it.iter.Valid() {
		return nil
	}
	row := it.schema.NewRow()
	if err := row.DecodeKey(it.schema, it.iter.Key()); err != nil {
		if err == ErrOutOfRange {
			return nil // ran off the end of this table's keyspace
		}
		return err
	}
	if err := row.DecodeVal(it.schema, it.iter.Val()); err != nil {
		return err
	}
	it.row = row
	it.valid = true
	return nil
}

// TableStartKey is the smallest possible KV key for a table — where a full scan
// begins.
func TableStartKey(schema *Schema) []byte {
	return append([]byte(schema.Table), 0x00)
}

// Seek returns a RowIterator over schema, positioned at the first row whose KV
// key is >= key.
func (db *DB) Seek(schema *Schema, key []byte) (*RowIterator, error) {
	kvit, err := db.KV.Seek(key)
	if err != nil {
		return nil, err
	}
	return NewRowIterator(schema, kvit)
}

// Scan returns a RowIterator over every row of schema, in key order.
func (db *DB) Scan(schema *Schema) (*RowIterator, error) {
	return db.Seek(schema, TableStartKey(schema))
}
