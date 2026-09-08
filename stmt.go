package db

// NamedCell is a "column = value" pair, as it appears in a WHERE clause.
type NamedCell struct {
	column string
	value  Cell
}

// StmtSelect is a parsed `select a,b from t where c=1 and d='e'`. The WHERE
// clause is restricted (for now) to "col = value AND ..." — a fixed shape that
// Chapter 5 replaces with a general expression.
type StmtSelect struct {
	table string
	cols  []string
	keys  []NamedCell
}
