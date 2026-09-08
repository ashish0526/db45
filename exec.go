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
		r.Header, r.Values, err = db.execSelect(s)
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

// checkExprCols walks an expression tree and errors on any column reference that
// is not in the schema — so `select nope from t` fails even when t is empty.
func checkExprCols(schema *Schema, expr interface{}) error {
	switch e := expr.(type) {
	case string:
		if schema.colIndex(e) < 0 {
			return fmt.Errorf("no such column %q", e)
		}
	case *Cell:
	case *ExprBinOp:
		if err := checkExprCols(schema, e.left); err != nil {
			return err
		}
		return checkExprCols(schema, e.right)
	case *ExprUnOp:
		return checkExprCols(schema, e.kid)
	}
	return nil
}

// exprLabel names an output column for the result header.
func exprLabel(expr interface{}, i int) string {
	if name, ok := expr.(string); ok {
		return name
	}
	return fmt.Sprintf("col%d", i+1)
}

// projectRow evaluates each output expression against a full row.
func projectRow(schema *Schema, exprs []interface{}, full Row) (Row, error) {
	out := make(Row, len(exprs))
	for i, e := range exprs {
		c, err := evalExpr(schema, full, e)
		if err != nil {
			return nil, err
		}
		out[i] = *c
	}
	return out, nil
}

func (db *DB) execSelect(s *StmtSelect) ([]string, []Row, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return nil, nil, err
	}
	header := make([]string, len(s.cols))
	for i, e := range s.cols {
		if err := checkExprCols(schema, e); err != nil {
			return nil, nil, err
		}
		header[i] = exprLabel(e, i)
	}

	it, err := db.execCond(schema, s.cond)
	if err != nil {
		return nil, nil, err
	}
	var rows []Row
	for ; it.Valid(); it.Next() {
		proj, err := projectRow(schema, s.cols, it.Row())
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, proj)
	}
	return header, rows, nil
}

// matchingRows collects every full row a WHERE expression selects, before any
// mutation — so an UPDATE/DELETE does not disturb the iterator it is walking.
func (db *DB) matchingRows(schema *Schema, cond interface{}) ([]Row, error) {
	it, err := db.execCond(schema, cond)
	if err != nil {
		return nil, err
	}
	var rows []Row
	for ; it.Valid(); it.Next() {
		rows = append(rows, it.Row())
	}
	return rows, nil
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
	// Validate the SET targets once.
	idxs := make([]int, len(s.value))
	for i, asn := range s.value {
		idx := schema.colIndex(asn.column)
		if idx < 0 {
			return 0, fmt.Errorf("no such column %q", asn.column)
		}
		if schema.isPKey(idx) {
			return 0, errors.New("UPDATE cannot change a primary key column")
		}
		idxs[i] = idx
	}

	rows, err := db.matchingRows(schema, s.cond)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		// Evaluate every RHS against the OLD row first, then assign — so
		// `SET a = b, b = a` swaps rather than cascades.
		newVals := make([]Cell, len(s.value))
		for i, asn := range s.value {
			c, err := evalExpr(schema, row, asn.expr)
			if err != nil {
				return 0, err
			}
			if c.Type != schema.Cols[idxs[i]].Type {
				return 0, fmt.Errorf("type mismatch assigning to %q", asn.column)
			}
			newVals[i] = *c
		}
		for i := range s.value {
			row[idxs[i]] = newVals[i]
		}
		if _, err := db.Update(schema, row); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (db *DB) execDelete(s *StmtDelete) (int, error) {
	schema, err := db.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	rows, err := db.matchingRows(schema, s.cond)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		ok, err := db.Delete(schema, row)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}
