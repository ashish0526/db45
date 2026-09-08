package sql

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ashish0526/db45/table"
)

// Engine is the SQL front end: it parses statements, keeps the system catalog
// (table schemas, themselves stored as data under a reserved KV key), and drives
// the table-level CRUD + range machinery.
type Engine struct {
	DB     *table.DB
	tables map[string]*table.Schema
}

// NewEngine wraps a table.DB. Call Open before use.
func NewEngine(db *table.DB) *Engine {
	return &Engine{DB: db, tables: map[string]*table.Schema{}}
}

// Open opens the underlying store and resets the schema cache.
func (e *Engine) Open() error {
	e.tables = map[string]*table.Schema{}
	return e.DB.Open()
}

// Close closes the underlying store.
func (e *Engine) Close() error { return e.DB.Close() }

// SQLResult is what executing one statement produces.
type SQLResult struct {
	Updated int         // rows affected (INSERT / UPDATE / DELETE)
	Header  []string    // column names (SELECT)
	Values  []table.Row // result rows, projected to Header (SELECT)
}

const schemaKeyPrefix = "@schema_"

// GetSchema returns a table's schema, reading it from the system catalog (a
// reserved KV key holding JSON) on a cache miss. "The database describes itself
// in its own tables" — real engines call this information_schema / sqlite_master.
func (e *Engine) GetSchema(name string) (*table.Schema, error) {
	if s, ok := e.tables[name]; ok {
		return s, nil
	}
	val, ok, err := e.DB.KV.Get([]byte(schemaKeyPrefix + name))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("table %q is not found", name)
	}
	s := &table.Schema{}
	if err := json.Unmarshal(val, s); err != nil {
		return nil, err
	}
	e.tables[name] = s
	return s, nil
}

// Exec parses and executes one SQL statement.
func (e *Engine) Exec(sql string) (SQLResult, error) {
	stmt, err := NewParser(sql).ParseStmt()
	if err != nil {
		return SQLResult{}, err
	}
	return e.ExecStmt(stmt)
}

// ExecStmt executes an already-parsed statement.
func (e *Engine) ExecStmt(stmt interface{}) (r SQLResult, err error) {
	switch s := stmt.(type) {
	case *StmtCreatTable:
		err = e.execCreateTable(s)
	case *StmtSelect:
		r.Header, r.Values, err = e.execSelect(s)
	case *StmtInsert:
		r.Updated, err = e.execInsert(s)
	case *StmtUpdate:
		r.Updated, err = e.execUpdate(s)
	case *StmtDelete:
		r.Updated, err = e.execDelete(s)
	default:
		panic("ExecStmt: unreachable")
	}
	return r, err
}

func (e *Engine) execCreateTable(s *StmtCreatTable) error {
	if _, ok, _ := e.DB.KV.Get([]byte(schemaKeyPrefix + s.table)); ok {
		return fmt.Errorf("table %q already exists", s.table)
	}
	schema := &table.Schema{Table: s.table, Cols: s.cols}
	for _, name := range s.pkey {
		idx := schema.ColIndex(name)
		if idx < 0 {
			return fmt.Errorf("primary key column %q is not a column", name)
		}
		schema.PKey = append(schema.PKey, idx)
	}
	blob, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	if _, err := e.DB.KV.Set([]byte(schemaKeyPrefix+s.table), blob); err != nil {
		return err
	}
	e.tables[s.table] = schema
	return nil
}

// checkExprCols walks an expression tree and errors on any column reference that
// is not in the schema — so `select nope from t` fails even when t is empty.
func checkExprCols(schema *table.Schema, expr interface{}) error {
	switch ex := expr.(type) {
	case string:
		if schema.ColIndex(ex) < 0 {
			return fmt.Errorf("no such column %q", ex)
		}
	case *table.Cell:
	case *ExprBinOp:
		if err := checkExprCols(schema, ex.left); err != nil {
			return err
		}
		return checkExprCols(schema, ex.right)
	case *ExprUnOp:
		return checkExprCols(schema, ex.kid)
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
func projectRow(schema *table.Schema, exprs []interface{}, full table.Row) (table.Row, error) {
	out := make(table.Row, len(exprs))
	for i, ex := range exprs {
		c, err := evalExpr(schema, full, ex)
		if err != nil {
			return nil, err
		}
		out[i] = *c
	}
	return out, nil
}

func (e *Engine) execSelect(s *StmtSelect) ([]string, []table.Row, error) {
	schema, err := e.GetSchema(s.table)
	if err != nil {
		return nil, nil, err
	}
	header := make([]string, len(s.cols))
	for i, ex := range s.cols {
		if err := checkExprCols(schema, ex); err != nil {
			return nil, nil, err
		}
		header[i] = exprLabel(ex, i)
	}

	it, err := execCond(e.DB, schema, s.cond)
	if err != nil {
		return nil, nil, err
	}
	var rows []table.Row
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
func (e *Engine) matchingRows(schema *table.Schema, cond interface{}) ([]table.Row, error) {
	it, err := execCond(e.DB, schema, cond)
	if err != nil {
		return nil, err
	}
	var rows []table.Row
	for ; it.Valid(); it.Next() {
		rows = append(rows, it.Row())
	}
	return rows, nil
}

func (e *Engine) execInsert(s *StmtInsert) (int, error) {
	schema, err := e.GetSchema(s.table)
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
	ok, err := e.DB.Insert(schema, row)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("duplicate primary key")
	}
	return 1, nil
}

func (e *Engine) execUpdate(s *StmtUpdate) (int, error) {
	schema, err := e.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	// Validate the SET targets once.
	idxs := make([]int, len(s.value))
	for i, asn := range s.value {
		idx := schema.ColIndex(asn.column)
		if idx < 0 {
			return 0, fmt.Errorf("no such column %q", asn.column)
		}
		if schema.IsPKey(idx) {
			return 0, errors.New("UPDATE cannot change a primary key column")
		}
		idxs[i] = idx
	}

	rows, err := e.matchingRows(schema, s.cond)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		// Evaluate every RHS against the OLD row first, then assign — so
		// `SET a = b, b = a` swaps rather than cascades.
		newVals := make([]table.Cell, len(s.value))
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
		if _, err := e.DB.Update(schema, row); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (e *Engine) execDelete(s *StmtDelete) (int, error) {
	schema, err := e.GetSchema(s.table)
	if err != nil {
		return 0, err
	}
	rows, err := e.matchingRows(schema, s.cond)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		ok, err := e.DB.Delete(schema, row)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}
