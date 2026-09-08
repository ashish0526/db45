package db

// NamedCell is a "column = value" pair, as it appears in a WHERE or SET clause.
type NamedCell struct {
	column string
	value  Cell
}

// The parsed statement types. parseStmt returns one of these via interface{};
// the executor switches on the concrete type (Go keeps the type tag, unlike a
// C void*).

// StmtSelect is `select a,b from t where c=1 and d='e'`. The WHERE clause is
// restricted (for now) to "col = value AND ..." — Chapter 5 generalises it.
type StmtSelect struct {
	table string
	cols  []string
	keys  []NamedCell
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

// StmtUpdate is `update t set a=1, b='x' where k=2`.
type StmtUpdate struct {
	table string
	keys  []NamedCell
	value []NamedCell
}

// StmtDelete is `delete from t where k=2`.
type StmtDelete struct {
	table string
	keys  []NamedCell
}
