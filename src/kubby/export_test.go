package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSensitiveExportTightensExistingFileBeforeReplacingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.txt")
	if err := os.WriteFile(path, []byte("old longer content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateText(path, "synthetic"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("export permissions %v", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "synthetic" {
		t.Fatal("export was not replaced correctly")
	}
}
