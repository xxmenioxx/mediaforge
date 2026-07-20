package applog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingWriterRetainsPreviousLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxLogSize); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	w := &rotatingWriter{path: path}
	if _, err := w.Write([]byte("new log\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated backup is missing: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new log\n" {
		t.Fatalf("new log content=%q", content)
	}
}
