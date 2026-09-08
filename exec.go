package db

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SQLResult is what executing one statement produces.
type SQLResult struct {
	Updated int      // rows affected (INSERT / UPDATE / DELETE)
	Header  []string // column names (SELECT)
	Values  []Row    // result rows, projected to Header (SELECT)
}

const schemaKeyPrefix = "@schema_"

// GetSchema returns a table's schema, reading it from the system catalog (a
// reserved KV key holding JSON) on a cache miss. "The database describes itself
// in its own tables" — real engines call this information_schema / sqlite_master.
func (db *DB) GetSchema(table string) (*Schema, error) {
	if s, ok := db.tables[table]; ok {
		return s, nil
	}
	val, ok, err := db.KV.Get([]byte(schemaKeyPrefix + table))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("table %q is not found", table)
	}
	s := &Schema{}
	if err := json.Unmarshal(val, s); err != nil {
		return nil, err
	}
	db.tables[table] = s
	return s, nil
}

// Exec parses and executes one SQL statement.
func (db *DB) Exec(sql string) (SQLResult, error) {
	stmt, err := NewParser(sql).ParseStmt()
	if err != nil {
		return SQLResult{}, err
	}
	return db.ExecStmt(stmt)
}

// ExecStmt executes an already-parsed statement.
func (db *DB) ExecStmt(stmt interface{}) (r SQLResult, err error) {
	switch s := stmt.(type) {
	case *StmtCreatTable:
		err = db.execCreateTable(s)
	case *StmtSelect:
		r.Header = s.cols
		r.Values, err = db.execSelect(s)
	case *StmtInsert:
		r.Updated, err = db.execInsert(s)
	case *StmtUpdate:
		r.Updated, err = db.execUpdate(s)
	case *StmtDelete:
		r.Updated, err = db.execDelete(s)
	default:
		panic("ExecStmt: unreachable")
	}
	return r, err
}

func (db *DB) execCreateTable(s *StmtCreatTable) error {
	if _, ok, _ := db.KV.Get([]byte(schemaKeyPrefix + s.table)); ok {
		return fmt.Errorf("table %q already exists", s.table)
	}
	schema := &Schema{Table: s.table, Cols: s.cols}
	for _, name := range s.pkey {
		idx := schema.colIndex(name)
		if idx < 0 {
			return fmt.Errorf("primary key column %q is not a column", name)
		}
		schema.PKey = append(schema.PKey, idx)
	}
	blob, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	if _, err := db.KV.Set([]byte(schemaKeyPrefix+s.table), blob); err != nil {
		return err
	}
	db.tables[s.table] = schema
	return nil
}

// lookupColumns resolves selected column names to indices in schema.Cols.
func lookupColumns(schema *Schema, names []string) ([]int, error) {
	idxs := make([]int, len(names))
	for i, name := range names {
		if idxs[i] = schema.colIndex(name); idxs[i] < 0 {
			return nil, fmt.Errorf("no such column %q", name)
		}
	}
	return idxs, nil
}

// makePKey checks that the WHERE equalities exactly cover the primary key and
// returns a Row with those key cells filled.
func makePKey(schema *Schema, keys []NamedCell) (Row, error) {
	if len(keys) != len(schema.PKey) {
		return nil, errors.New("WHERE must constrain exactly the primary key")
	}
	row := schema.NewRow()
	for _, pk := range schema.PKey {
		name := schema.Cols[pk].Name
		found := false
		for _, nc := range keys {
			if nc.column == name {
				row[pk] = nc.value
				row[pk].Type = schema.Cols[pk].Type
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("primary key column %q not constrained", name)
		}
	}
	return row, nil
}

// subsetRow projects a full row down to the columns at idxs.
func subsetRow(row Row, idxs []int) Row {
	out := make(Row, len(idxs))
	for i, idx := range idxs {
		out[i] = row[idx]
	}
	return out
}

func (db *DB) execSelect(s *StmtSelect) ([]Row, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return nil, err
	}
	idxs, err := lookupColumns(schema, s.cols)
	if err != nil {
		return nil, err
	}
	row, err := makePKey(schema, s.keys)
	if err != nil {
		return nil, err
	}
	ok, err := db.Select(schema, row)
	if err != nil || !ok {
		return nil, err
	}
	return []Row{subsetRow(row, idxs)}, nil
}

func (db *DB) execInsert(s *StmtInsert) (int, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	if len(s.value) != len(schema.Cols) {
		return 0, fmt.Errorf("INSERT has %d values, table has %d columns", len(s.value), len(schema.Cols))
	}
	row := schema.NewRow()
	for i := range s.value {
		row[i] = s.value[i]
		row[i].Type = schema.Cols[i].Type
	}
	ok, err := db.Insert(schema, row)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("duplicate primary key")
	}
	return 1, nil
}

func (db *DB) execUpdate(s *StmtUpdate) (int, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	row, err := makePKey(schema, s.keys)
	if err != nil {
		return 0, err
	}
	ok, err := db.Select(schema, row)
	if err != nil || !ok {
		return 0, err
	}
	for _, asn := range s.value {
		idx := schema.colIndex(asn.column)
		if idx < 0 {
			return 0, fmt.Errorf("no such column %q", asn.column)
		}
		if schema.isPKey(idx) {
			return 0, errors.New("UPDATE cannot change a primary key column")
		}
		row[idx] = asn.value
		row[idx].Type = schema.Cols[idx].Type
	}
	if _, err := db.Update(schema, row); err != nil {
		return 0, err
	}
	return 1, nil
}

func (db *DB) execDelete(s *StmtDelete) (int, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	row, err := makePKey(schema, s.keys)
	if err != nil {
		return 0, err
	}
	ok, err := db.Delete(schema, row)
	if err != nil || !ok {
		return 0, err
	}
	return 1, nil
}
