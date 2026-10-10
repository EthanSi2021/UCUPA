package store

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionStoreSourceContainsOnlyCurrentMarker(t *testing.T) {
	const expectedMarker = "ucupa.core.local@9c4f2a6e8b1d7350f6a3c9e2b7d4f1a8"
	if FormatMarker != expectedMarker {
		t.Fatalf("current marker = %q, want %q", FormatMarker, expectedMarker)
	}
	// Scan the whole owning module, not only the old root and Store folder:
	// moving another production file must not hide a second format marker.
	var production []byte
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		production = append(production, encoded...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := bytes.Count(production, []byte(FormatMarker)); count != 1 {
		t.Fatalf("current marker occurrence count = %d, want 1", count)
	}
	if count := bytes.Count(production, []byte("ucupa.core.local@")); count != 1 {
		t.Fatalf("production Store marker prefix occurrence count = %d, want 1", count)
	}
}
