package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCapabilityUsesRawPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capability")
	if err := os.WriteFile(path, []byte("raw-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := readCapability(path)
	if err != nil || token != "raw-secret" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCapability(path); err == nil {
		t.Fatal("empty capability accepted")
	}
}
