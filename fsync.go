//go:build unix

package db

import (
	"os"
	"path/filepath"
	"syscall"
)

// createFileSync creates (or opens) file for read+write and fsyncs the parent
// directory, so the file's *existence* — not just its future contents — is
// durable. On Unix, fsync(file) flushes the file's data but not the directory
// entry that records the file; creating, renaming or deleting a file therefore
// also needs an fsync on the containing directory.
func createFileSync(file string) (*os.File, error) {
	fp, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err = syncDir(file); err != nil {
		_ = fp.Close()
		return nil, err
	}
	return fp, nil
}

// syncDir fsyncs the directory that contains file. A directory file descriptor
// can be fsynced even though it cannot be read like a regular file.
func syncDir(file string) error {
	dirfd, err := syscall.Open(filepath.Dir(file), os.O_RDONLY|syscall.O_DIRECTORY, 0o644)
	if err != nil {
		return err
	}
	defer syscall.Close(dirfd)
	return syscall.Fsync(dirfd)
}
