package db

import (
	"io"
	"os"
)

// Log is the write-ahead log: an append-only file of every state change. Entries
// are added at the end, never modified or deleted; replaying them front-to-back
// reconstructs the current state (the last write to a key wins). This gives
// incremental durability — a small append per change instead of rewriting the
// whole dataset.
type Log struct {
	FileName string
	fp       *os.File
}

// Open creates the log file if it does not exist and opens it for read+write,
// fsyncing the parent directory so the file itself is durable.
func (log *Log) Open() (err error) {
	log.fp, err = createFileSync(log.FileName)
	return err
}

// Close closes the underlying file.
func (log *Log) Close() error {
	if log.fp == nil {
		return nil
	}
	return log.fp.Close()
}

// Write appends one encoded Entry and fsyncs the file. The write is not durable
// until Sync returns: plain write(2) only copies bytes into the OS page cache,
// which a power cut loses. fsync is slow (a real disk round-trip) — every Set
// pays one here; real databases batch writes into one fsync (group commit).
func (log *Log) Write(ent *Entry) error {
	if _, err := log.fp.Write(ent.Encode()); err != nil {
		return err
	}
	return log.fp.Sync()
}

// Truncate empties the log and rewinds it — called after a compaction has folded
// the log's contents into an SSTable.
func (log *Log) Truncate() error {
	if err := log.fp.Truncate(0); err != nil {
		return err
	}
	if _, err := log.fp.Seek(0, 0); err != nil {
		return err
	}
	return log.fp.Sync()
}

// Read decodes the next Entry from the current file position. eof reports the end
// of usable log: a clean end (io.EOF), or an incomplete final record — a torn
// header/body (io.ErrUnexpectedEOF) or a checksum mismatch (ErrBadSum) — which is
// silently dropped, since damage is always confined to the tail of an
// append-only file.
func (log *Log) Read(ent *Entry) (eof bool, err error) {
	err = ent.Decode(log.fp)
	switch err {
	case nil:
		return false, nil
	case io.EOF, io.ErrUnexpectedEOF, ErrBadSum:
		return true, nil
	default:
		return false, err
	}
}
