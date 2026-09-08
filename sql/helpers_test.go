package sql

import (
	"testing"

	"github.com/ashish0526/db45/table"
)

// openDB returns an opened SQL Engine over a fresh temp directory.
func openDB(t *testing.T) *Engine { return openDBDir(t, t.TempDir()) }

// openDBDir opens an Engine over a specific directory (for restart tests).
func openDBDir(t *testing.T, dir string) *Engine {
	t.Helper()
	e := NewEngine(&table.DB{})
	e.DB.KV.Options.Dirpath = dir
	if err := e.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func linkSchema() *table.Schema {
	return &table.Schema{
		Table: "link",
		Cols: []table.Column{
			{Name: "time", Type: table.TypeI64},
			{Name: "src", Type: table.TypeStr},
			{Name: "dst", Type: table.TypeStr},
		},
		PKey: []int{1, 2},
	}
}

func mkRow(schema *table.Schema, tm int64, src, dst string) table.Row {
	r := schema.NewRow()
	r[0] = table.Cell{Type: table.TypeI64, I64: tm}
	r[1] = table.Cell{Type: table.TypeStr, Str: []byte(src)}
	r[2] = table.Cell{Type: table.TypeStr, Str: []byte(dst)}
	return r
}
