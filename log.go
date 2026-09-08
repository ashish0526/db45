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

// Open creates the log file if it does not exist and opens it for read+write.
func (log *Log) Open() (err error) {
	log.fp, err = os.OpenFile(log.FileName, os.O_RDWR|os.O_CREATE, 0o644)
	return err
}

// Close closes the underlying file.
func (log *Log) Close() error {
	if log.fp == nil {
		return nil
	}
	return log.fp.Close()
}

// Write appends one encoded Entry to the end of the log.
func (log *Log) Write(ent *Entry) error {
	_, err := log.fp.Write(ent.Encode())
	return err
}

// Read decodes the next Entry from the current file position. eof reports a clean
// end of the log.
func (log *Log) Read(ent *Entry) (eof bool, err error) {
	err = ent.Decode(log.fp)
	if err == io.EOF {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return false, nil
}
