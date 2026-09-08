# db45 — code a database in 45 steps (Go)

A worked, **commit-by-commit** implementation of the *Trial of Code* series
["Code a Database in 45 Steps"](https://trialofcode.org/database/): a small
relational SQL database built as a set of data structures layered over a sorted
key/value store, ending in a working **LSM-Tree** storage engine.

Every numbered step is a single commit that leaves `go test ./...` green. Reading
`git log --reverse` (or the **Commits** tab on GitHub) walks you from an in-memory
`map` to an on-disk log-structured merge tree with a SQL front end.

The 45 steps are built as **one package** (that is how the course teaches it — one
program, growing). The final commits then split that package into three layers —
`storage/` (the LSM engine), `table/` (schema + rows over KV), `sql/` (parser,
interpreter, planner) — so the finished code reads like a real project. To follow
a single step, check that commit out; to use the library, import the packages.

```
in-memory map → serialize records → append-only log → fsync → per-record checksum
→ typed cells → table schema, rows as KV → insert/update/upsert → CRUD
→ SQL tokenizer → value literals → parse SELECT → all statements → execute + catalog
→ sorted array + binary search → iterators → order-preserving key encoding
→ row iterator → Range + prefix bounds → expression tree → tree-walking interpreter
→ operator precedence → full expression grammar → expressions in SELECT/UPDATE
→ WHERE as an expression → SQL range queries → SSTable file format → query an SSTable
→ refactor the MemTable → k-way merge → tombstones + compaction
→ atomic metadata (double buffering) → directory layout → multiple levels
→ compaction policy → (indexes) → (MVCC / concurrency)
```

## How to read it

Each commit is a lesson. Review them in order:

```bash
git clone <this repo>
cd db45
git log --reverse --oneline          # the 45 steps, oldest first
git show <hash>                       # one step's code + a note on what it teaches
```

On GitHub: open the **Commits** list and click each `Step NNNN — …` commit to see
its diff and the explanation in the commit message.

To build/run at a specific step:

```bash
git checkout <hash>              # detached HEAD at that step
go test ./...
go test ./... -run TestEntry -v  # watch that step's behaviour
git checkout main               # back to the finished engine
```

At most of the 45 steps the code is still one flat package; the layered
`storage/ table/ sql/` layout only appears in the last few commits.

## The 45 steps

| Step | What you build | The idea behind it |
| --- | --- | --- |
| 0000 | project scaffold | (course setup steps 0001–0002 — no checkpoint) |
| **Chapter 1 — a log-based key/value store** | | *durability from an append-only log* |
| 0101 | `KV` over a Go map: `Open/Close/Get/Set/Del` | where this DB sits: KV, OLTP, disk-based; ACID as four separate things |
| 0102 | `Entry.Encode` / `Decode(io.Reader)` | length-prefixed binary records; short read vs clean EOF |
| 0103 | append-only `Log`, replayed on `Open` | the write-ahead log; last write to a key wins |
| 0104 | `fsync` after every append (+ directory fsync) | the page cache; a write isn't durable until `fsync` returns |
| 0105 | `crc32` per record; drop a torn tail | writes aren't atomic across a power cut; detectably-corrupt |
| **Chapter 2 — tables on top of KV** | | *primary-key columns → KV key, the rest → KV value* |
| 0201 | `Cell`: `int64` or `[]byte`, encode/decode | the minimal type basis; endianness; two's complement |
| 0202 | `Schema` / `Row`; row ↔ KV key + value | the primary key *is* the KV key; the table prefix |
| 0203 | `SetEx` with INSERT / UPDATE / UPSERT modes | `Set` was quietly an upsert; "did a write happen?" |
| 0204 | primary-key `Select/Insert/Upsert/Update/Delete` | the relational layer is a thin wrapper over KV |
| **Chapter 3 — a tiny SQL engine** | | *tokenize → parse → execute; no compiler theory* |
| 0301 | tokenizer primitives (`tryName`, `tryKeyword`, …) | `try*` (speculative) vs `parse*` (commits); lexing vs parsing |
| 0302 | `parseValue` → `Cell` | predictive parsing: dispatch on the first character |
| 0303 | parse `SELECT … WHERE col = v AND …` | a comma list and an `AND` list are the same loop |
| 0304 | `CREATE TABLE` / `INSERT` / `UPDATE` / `DELETE` | heterogeneous statements via `interface{}` |
| 0305 | execute statements; the system catalog | the schema is itself data, stored under a reserved key |
| **Chapter 4 — range queries** | | *iterate keys in sorted order between two bounds* |
| 0401 | sorted parallel slices + binary search | binary search returns an insertion point; `O(N)` writes on purpose |
| 0402 | `Seek` / `Valid` / `Key` / `Val` / `Next` / `Prev` | why an iterator, not a result slice; the "one past each end" trick |
| 0403 | order-preserving key encoding | big-endian + sign-bit flip; `0x00`-terminated escaped strings; tuple order |
| 0404 | `RowIterator` — decoded rows, stops at the table | a full scan is a range query with the widest bounds |
| 0405 | `DB.Range` + prefix bounds | reduce a multi-column comparison to one byte-string comparison (±∞ sentinels) |
| **Chapter 5 — real SQL expressions** | | *an expression tree + a recursive interpreter* |
| 0501 | `ExprBinOp` tree; parse `+ -` | three node kinds via `interface{}`; left-associativity from a loop |
| 0502 | `evalExpr(schema, row, expr)` | a tree-walking interpreter; comparisons yield a boolean cell |
| 0503 | operator precedence; parentheses | one parse function per precedence level; parens loop back to the top |
| 0504 | full grammar: `OR AND NOT = <> < > … + - * /` unary `-` | a precedence table drives a generic parser (a readable Pratt parser) |
| 0505 | expressions in `SELECT` / `UPDATE SET` | assignment RHS is evaluated against the *old* row |
| 0506 | `WHERE` becomes one expression | query analysis in miniature: pattern-match a servable shape |
| 0507 | SQL range queries; `(a,b) > (1,2)` tuples | a point lookup is a zero-width range; `OR` needs a union (stops here) |
| **Chapter 6 — data on disk (SSTables)** | | *never edit a big sorted file — merge into a new one* |
| 0600 | design space (DESIGN.md) | copy-on-write / double-buffering / physical logging / LSM |
| 0601 | build an SSTable from a sorted iterator | the offset array is an on-disk sorted index; single-pass `WriteAt` |
| 0602 | query an SSTable (`index`, binary search, iterator) | positioned I/O (`pread`/`pwrite`); return interfaces, not structs |
| 0603 | extract `SortedArray` implementing `SortedKV` | scaffolding: access the structure only through methods |
| 0604 | k-way merge of sorted levels | merge sort is the heart of an LSM read; top level wins ties |
| 0605 | MemTable + 1 SSTable; `Compact`; tombstones | you can't delete from a lower level, so you mark; `rename` is atomic |
| **Chapter 7 — the LSM-Tree** | | *track a changing set of files atomically* |
| 0700 | what an LSM-Tree is (DESIGN.md) | a *method*, not a structure: merge sorted runs instead of updating |
| 0701 | double-buffered atomic metadata store | two slots, alternate writes, higher valid version wins |
| 0702 | the DB owns a directory; commit by metadata pointer | a crash mid-compaction just leaves an orphan file |
| 0703 | `main []SortedFile`; metadata `[]string` | the read path barely changed — the merge was built for *k* levels |
| 0704 | compaction policy (size-tiered merge) | tuning the knobs trades write vs read amplification vs space |
| **Chapters 8–9 — book only** | | see [`DESIGN.md`](DESIGN.md) |
| — | secondary indexes; the B+Tree alternative | an index is another KV keyspace `cols → primary-key` |
| — | transactions, locking, MVCC, snapshot isolation | MVCC fits an LSM: tag writes with a seqno, snapshot on read |

The free site publishes 39 pages (2 setup + 37 numbered steps); this repo
implements all 37 as commits `Step 0101` … `Step 0704`. Chapters 8–9 (the
remaining "45") are book-only and covered conceptually in `DESIGN.md`.

## Package layout

```
storage/   the LSM storage engine — []byte keys and values only
           KV, KVOptions, Get/Set/Del/SetEx/Seek/Range/Compact
           log, SSTable, k-way merge, tombstone filter, atomic metadata

table/     the relational layer over storage
           Cell (int64 / []byte), Schema, Row, order-preserving key codecs
           DB — primary-key Select/Insert/Upsert/Update/Delete
           RowIterator — decoded rows, stops at the table boundary

sql/       the SQL front end over table
           Parser (tokenizer + recursive descent), expression grammar
           evalExpr (tree-walking interpreter)
           Engine — Exec(sql) + the system catalog
           planner — recognise a WHERE shape, pick an access path

examples/basic/   a runnable end-to-end demo
```

Import direction is a straight line: `sql → table → storage`.

## Try it

```go
package main

import (
	"fmt"

	"github.com/ashish0526/db45/sql"
	"github.com/ashish0526/db45/table"
)

func main() {
	db := &table.DB{}
	db.KV.Options.Dirpath = "./data" // the engine owns this directory

	eng := sql.NewEngine(db)
	must(eng.Open())
	defer eng.Close()

	eng.Exec(`create table link (t int64, src string, dst string, primary key (src, dst))`)
	eng.Exec(`insert into link values (1700, 'alice', 'bob')`)
	eng.Exec(`insert into link values (1701, 'alice', 'carol')`)

	r, _ := eng.Exec(`select src, dst, t from link where (src, dst) >= ('alice', 'bob')`)
	for _, row := range r.Values {
		fmt.Printf("%s -> %s @%d\n", row[0].Str, row[1].Str, row[2].I64)
	}

	db.KV.Compact() // fold the MemTable into an SSTable
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
```

Run the full demo with `go run ./examples/basic`.

Supported SQL: `CREATE TABLE` (`int64` / `string` columns, `primary key (...)`),
`INSERT INTO … VALUES (...)`, `SELECT <exprs> FROM t [WHERE <expr>]`,
`UPDATE t SET col = <expr>, … [WHERE <expr>]`, `DELETE FROM t [WHERE <expr>]`.
`WHERE` supports point lookups on the whole primary key (`a = 1 AND b = 2`),
ranges over a primary-key prefix (`a > 1`, `a >= 1 AND a < 9`, `(a,b) > (1,2)`),
and full scans (no `WHERE`).

## Running the tests

```bash
go test ./...                    # everything
go test ./... -run TestEntry -v  # one step's tests, verbosely
go test ./... -count=1           # skip the result cache
```

Requires Go 1.21+ (uses the standard-library `slices` package).

## Source layout

**`storage/`**

| file | role |
| --- | --- |
| `kv.go` | the engine: MemTable + k SSTable levels, `Get/Set/Del`, `Compact` |
| `sortedarray.go` | the in-memory sorted MemTable (with tombstones) and its cursor |
| `entry.go` | WAL record format — length-prefix, deleted flag, crc32 |
| `log.go` | append-only write-ahead log: write, replay, truncate |
| `fsync.go` | `createFileSync` / `syncDir` — durable file creation |
| `sstable.go` | immutable sorted files on disk (offset-array index, positioned I/O) |
| `merge.go` | k-way merge of sorted levels, reversible mid-iteration |
| `filterdel.go` | tombstone filtering for reads and last-level merges |
| `ranged.go` | `KV.Range` — a closed byte-key interval + direction |
| `meta.go` | double-buffered atomic metadata store (the SSTable level list) |

**`table/`**

| file | role |
| --- | --- |
| `cell.go` | typed values; value + order-preserving key codecs |
| `schema.go` | `Schema` / `Row`; rows ↔ KV key + value; key prefixes with ±∞ |
| `db.go` | primary-key CRUD |
| `rowiter.go` | `RowIterator` — decoded rows, stops at the table boundary |

**`sql/`**

| file | role |
| --- | --- |
| `parser.go`, `parse_value.go`, `parse_stmt.go` | tokenizer + recursive-descent statement parsers |
| `expr.go`, `eval.go` | expression grammar + tree-walking interpreter |
| `stmt.go` | parsed-statement structs |
| `engine.go` | `Engine.Exec` + system catalog (schemas stored as data) |
| `planner.go`, `match.go` | recognise a WHERE shape → pick an access path; `Range` |

## Credits

Course, lesson text and reference solutions: ***Trial of Code*** — Lowram Eepson,
<https://trialofcode.org/database/>. This repo is a study reimplementation
following that series; the tests here are original (the course's own test suites
are not reproduced). See `DESIGN.md` for the Chapter 6–9 design notes.
