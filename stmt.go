package db

// NamedCell is a "column = value" pair, as it appears in a WHERE or SET clause.
type NamedCell struct {
	column string
	value  Cell
}

// The parsed statement types. parseStmt returns one of these via interface{};
// the executor switches on the concrete type (Go keeps the type tag, unlike a
// C void*).

// StmtSelect is `select a*4-b, d+c from t where ...`. Output columns are now
// arbitrary expressions; the WHERE clause is still "col = value AND ..." until
// Step 0506.
type StmtSelect struct {
	table string
	cols  []interface{} // output expressions
	keys  []NamedCell
}

// ExprAssign is one `column = expression` in an UPDATE ... SET list.
type ExprAssign struct {
	column string
	expr   interface{}
}

// StmtCreatTable is `create table t (a int64, b string, primary key (a))`.
type StmtCreatTable struct {
	table string
	cols  []Column
	pkey  []string
}

// StmtInsert is `insert into t values (1, 'x')`.
type StmtInsert struct {
	table string
	value []Cell
}

// StmtUpdate is `update t set a = a-b, b = a where k=2`.
type StmtUpdate struct {
	table string
	keys  []NamedCell
	value []ExprAssign
}

// StmtDelete is `delete from t where k=2`.
type StmtDelete struct {
	table string
	keys  []NamedCell
}
