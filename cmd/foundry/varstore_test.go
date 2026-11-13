package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckVarstoreArgs(t *testing.T) {
	tests := []struct {
		profile, arch, wantErr string
	}{
		{profile: "my-keys", arch: "x86_64"},
		{profile: "my-keys", arch: "aarch64"},
		{profile: "my_keys", arch: "x86_64", wantErr: "profile name"},
		{profile: "my-keys", arch: "arm64", wantErr: "--arch"},
	}
	for _, tt := range tests {
		err := checkVarstoreArgs(tt.profile, tt.arch)
		if tt.wantErr == "" && err != nil {
			t.Errorf("checkVarstoreArgs(%q, %q) error = %v", tt.profile, tt.arch, err)
		}
		if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("checkVarstoreArgs(%q, %q) error = %v, want %q", tt.profile, tt.arch, err, tt.wantErr)
		}
	}
}

// varstoreBytes is a minimal raw edk2 varstore of one 4 KiB block.
func varstoreBytes() []byte {
	data := make([]byte, 4096)
	copy(data[16:32], []byte{0x8d, 0x2b, 0xf1, 0xff, 0x96, 0x76, 0x8b, 0x4c, 0xa9, 0x85, 0x27, 0x47, 0x07, 0x5b, 0x4f, 0x50})
	binary.LittleEndian.PutUint64(data[32:40], 4096)
	copy(data[40:44], "_FVH")
	return data
}

func TestReadVarstoreSource(t *testing.T) {
	want := varstoreBytes()
	path := filepath.Join(t.TempDir(), "vars.fd")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readVarstoreSource(path, nil)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("readVarstoreSource(file) = %d bytes, %v", len(got), err)
	}
	got, err = readVarstoreSource("-", bytes.NewReader(want))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("readVarstoreSource(-) = %d bytes, %v", len(got), err)
	}
	if _, err := readVarstoreSource("-", bytes.NewReader(make([]byte, 4096))); err == nil {
		t.Error("expected an error for a file that is not a varstore")
	}
	if _, err := readVarstoreSource(filepath.Join(t.TempDir(), "missing.fd"), nil); err == nil {
		t.Error("expected an error for a missing file")
	}
}
