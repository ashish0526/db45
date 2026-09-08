//go:build unix

package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFileSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "file")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	fp, err := createFileSync(path)
	if err != nil {
		t.Fatalf("createFileSync: %v", err)
	}
	defer fp.Close()

	if _, err := fp.WriteString("durable"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := fp.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := syncDir(path); err != nil {
		t.Fatalf("syncDir: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "durable" {
		t.Fatalf("read back: %q err=%v", got, err)
	}
}
