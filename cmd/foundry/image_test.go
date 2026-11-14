package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// "-" with a terminal (or another character device) as standard input is
// refused instead of waiting for typing.
func TestIsTerminal(t *testing.T) {
	if isTerminal(bytes.NewReader(nil)) {
		t.Error("a reader that is not a file is not a terminal")
	}
	regular := filepath.Join(t.TempDir(), "image.raw")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(regular)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if isTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
	dev, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s: %v", os.DevNull, err)
	}
	defer func() { _ = dev.Close() }()
	if !isTerminal(dev) {
		t.Errorf("%s is a character device", os.DevNull)
	}
}
