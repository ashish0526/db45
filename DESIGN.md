# Design notes

Chapter intros from the notebook that carry no code checkpoint of their own.

## Step 0600 — Atomic updates (Chapter 6 intro)

So far: log + in-memory sorted array = an in-memory DB with durability, capped by
RAM. The next chapters put the sorted data in a file.

An array is not a real data structure — `O(N)` insert/delete. The only practical
on-disk choices are the **B+Tree** and the **LSM-Tree**, and both can be reached
from "sorted array, improved". The hard part is *updating a disk structure
safely*; the toolbox:

- **copy-on-write** — never overwrite a node/page; write a new copy and re-point.
- **double buffering** — keep two copies of a small record, alternate writes; the
  one with a valid checksum and higher version wins after a crash (Step 0701).
- **physical logging** — log the byte-level page changes, not the logical op.
- **LSM-Tree** — replace *updating* a structure with *merging* sorted runs. The
  path this project takes.

The plan for Chapter 6: writes go to a small in-memory sorted array (the
**MemTable**); periodically the whole thing is merged into an immutable sorted
file (an **SSTable**) and the log is truncated. Editing a large sorted file in
place is slow and hard to make crash-safe, so you never edit it — you write a new
file that is the merge of (old file + recent changes) and atomically swap it in.

## Step 0700 — LSM-Tree introduction (Chapter 7 intro)

An **LSM-Tree** ("Log-structured Merge-tree") is a *method*, not a data
structure: replace *updating* a structure with *merging* sorted runs. The name
is misleading — no tree, no log required. B+Tree and LSM-Tree are the two ways
to build a general-purpose OLTP engine.

The structure is *k* stacked levels of sorted KV, smallest/newest/highest
priority at the top:

```
level 1   [x=1, z=3]
level 2   [a=8, y=2, z=(deleted)]
...
level k   [a=0, b=2, c=3, d=4, z=456]
```

- **Writes** land in level 1 (via the MemTable); when a level grows too big it
  merges *down* into the next.
- **Reads** consult levels top->down, first hit wins (point query), or k-way
  merge all levels (range query).
- Each level just has to be "a sorted set of KV" — so an LSM-Tree is a *family*
  of structures.

Trade-off vs B+Tree: fast sequential writes and good compression, but **write
amplification** (data is rewritten as it moves down) and slower reads (multiple
levels to check — mitigated with Bloom filters).

Chapter 7 generalises "MemTable + 1 SSTable" to "MemTable + k SSTables". The new
problems are all about **tracking a changing set of files atomically**.
