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
