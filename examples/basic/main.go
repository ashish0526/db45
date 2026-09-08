// Command basic is a tiny end-to-end demo of the db45 SQL engine.
package main

import (
	"fmt"
	"os"

	"github.com/ashish0526/db45/sql"
	"github.com/ashish0526/db45/table"
)

func main() {
	dir, err := os.MkdirTemp("", "db45-demo-")
	must(err)
	defer os.RemoveAll(dir)

	db := &table.DB{}
	db.KV.Options.Dirpath = dir // the engine owns this directory

	eng := sql.NewEngine(db)
	must(eng.Open())
	defer eng.Close()

	exec(eng, `create table link (t int64, src string, dst string, primary key (src, dst))`)
	exec(eng, `insert into link values (1700, 'alice', 'bob')`)
	exec(eng, `insert into link values (1701, 'alice', 'carol')`)
	exec(eng, `insert into link values (1702, 'bob', 'carol')`)

	// full scan, rows come back in primary-key order
	r, err := eng.Exec(`select src, dst, t from link`)
	must(err)
	fmt.Println("all links (src, dst, t):")
	for _, row := range r.Values {
		fmt.Printf("  %s -> %s  @%d\n", row[0].Str, row[1].Str, row[2].I64)
	}

	// point lookup by full primary key
	r, err = eng.Exec(`select t from link where src = 'alice' and dst = 'carol'`)
	must(err)
	fmt.Println("alice->carol at:", r.Values[0][0].I64)

	// range scan over a primary-key prefix
	r, err = eng.Exec(`select src, dst from link where (src, dst) > ('alice', 'bob')`)
	must(err)
	fmt.Println("links after (alice, bob):")
	for _, row := range r.Values {
		fmt.Printf("  %s -> %s\n", row[0].Str, row[1].Str)
	}

	res, err := eng.Exec(`update link set t = t + 1 where src = 'alice' and dst = 'bob'`)
	must(err)
	fmt.Printf("updated %d row(s)\n", res.Updated)

	// fold the in-memory MemTable into an on-disk SSTable
	must(db.KV.Compact())
	fmt.Println("compacted")
}

func exec(eng *sql.Engine, q string) {
	if _, err := eng.Exec(q); err != nil {
		panic(fmt.Sprintf("%q: %v", q, err))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
