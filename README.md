# db45 — a database in 45 steps (Go)

A worked, commit-by-commit implementation of the *Trial of Code* "Code a Database
in 45 Steps" project (<https://trialofcode.org/database/>), following the study
notebook in `../Database in 45 Steps (Go) — Study Notebook.md`.

Unlike the original course (which ships one directory per step), this repo is
**one evolving Go package**. Each numbered step is a single commit that edits the
package in place and leaves `go test ./...` green. Read `git log --reverse` to
watch a relational database grow out of an in-memory map.

## Layout

```
kv.go          storage engine: MemTable + k SSTable levels, Get/Set/Del, Compact
sortedarray.go the in-memory sorted MemTable (+ tombstones) and its cursor
entry.go       write-ahead-log record format (length-prefix, deleted flag, crc32)
log.go         append-only WAL: write, replay, truncate
fsync.go       createFileSync / syncDir — durable file creation
cell.go        typed values (int64 / []byte); value + order-preserving key codecs
table.go       Schema / Row; rows <-> KV key+value; key prefixes with ±inf
db.go          primary-key CRUD
exec.go        SQL executor + system catalog (schemas stored as data)
parser.go      SQL tokenizer + recursive-descent primitives
parse_value.go literal parsing (int / quoted string)
parse_stmt.go  statement parsers (SELECT / CREATE / INSERT / UPDATE / DELETE)
expr.go        expression grammar (full precedence ladder), ExprBinOp / ExprUnOp
eval.go        tree-walking expression interpreter
range.go       RangeReq / RangedKVIter / DB.Range (closed intervals, direction)
rowiter.go     RowIterator: decoded rows, stops at the table boundary
match.go       matchAllEq — recognise a point-lookup WHERE
makerange.go   makeRange — pick an access path (scan / point / prefix range)
sstable.go     immutable sorted files on disk (offset-array index, positioned I/O)
merge.go       k-way merge of sorted levels, reversible mid-iteration
filterdel.go   tombstone filtering for reads and last-level merges
meta.go        double-buffered atomic metadata store (the SSTable level list)
```

## The arc

```
Chapter 1  0101-0105  log-based key/value store   durability from an append-only log
Chapter 2  0201-0204  tables on top of KV         schema, rows encoded as KV
Chapter 3  0301-0305  a tiny SQL engine           tokenizer -> parser -> executor
Chapter 4  0401-0405  range queries               sorted order, iterators, ORDER BY
Chapter 5  0501-0507  real SQL expressions        an expression tree + interpreter
Chapter 6  0601-0605  data on disk                SSTables, k-way merge, compaction
Chapter 7  0701-0704  the LSM-Tree                many SSTable levels, atomic metadata
```

## Running

```
go test ./...
```

Requires Go 1.21+ (uses `slices` / `maps` from the standard library).
