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
kv.go        storage engine: Get / Set / Del over []byte keys
entry.go     on-disk record format (length-prefix, deleted flag, crc32)
log.go       append-only write-ahead log + replay
cell.go      typed values (int64 / []byte), value + order-preserving key encodings
table.go     Schema / Row, rows <-> KV pairs
db.go        primary-key CRUD, then the SQL engine + system catalog
parser.go    SQL tokenizer + recursive-descent parser
expr.go      expression tree + tree-walking interpreter
sstable.go   immutable sorted files on disk
merge.go     k-way merge of sorted levels
meta.go      double-buffered atomic metadata store
```

(Files appear as the steps that introduce them land.)

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
