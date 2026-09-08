package db

// DB is the relational layer: primary-key CRUD, each operation a thin wrapper
// over the KV storage engine. Chapter 3 adds a SQL front end on top of this.
type DB struct {
	KV KV
}

func (db *DB) Open() error  { return db.KV.Open() }
func (db *DB) Close() error { return db.KV.Close() }

// Select looks a row up by primary key. The caller fills row's primary-key cells;
// on a hit Select fills in the rest from the decoded value. ok reports a hit.
func (db *DB) Select(schema *Schema, row Row) (ok bool, err error) {
	key := row.EncodeKey(schema)
	val, ok, err := db.KV.Get(key)
	if err != nil || !ok {
		return false, err
	}
	if err := row.DecodeVal(schema, val); err != nil {
		return false, err
	}
	return true, nil
}

// Insert writes a complete row only if its primary key is not already present.
func (db *DB) Insert(schema *Schema, row Row) (updated bool, err error) {
	return db.KV.SetEx(row.EncodeKey(schema), row.EncodeVal(schema), ModeInsert)
}

// Upsert writes a complete row, overwriting any existing one.
func (db *DB) Upsert(schema *Schema, row Row) (updated bool, err error) {
	return db.KV.SetEx(row.EncodeKey(schema), row.EncodeVal(schema), ModeUpsert)
}

// Update rewrites a complete row only if its primary key already exists. It only
// ever changes V, never K — changing a primary key is a delete + insert.
func (db *DB) Update(schema *Schema, row Row) (updated bool, err error) {
	return db.KV.SetEx(row.EncodeKey(schema), row.EncodeVal(schema), ModeUpdate)
}

// Delete removes a row addressed by its primary-key cells.
func (db *DB) Delete(schema *Schema, row Row) (deleted bool, err error) {
	return db.KV.Del(row.EncodeKey(schema))
}
