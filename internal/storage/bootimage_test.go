package storage

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bootableRaw is a raw image with the boot sector signature, size bytes long.
func bootableRaw(size int) []byte {
	b := make([]byte, size)
	copy(b[510:], mbrSignature)
	return b
}

// TestCheckBootImageFile: a boot image given as a host path is checked as an
// import is, and its format comes from its content, not its name.
func TestCheckBootImageFile(t *testing.T) {
	dataFile := qcow2Fixture(4096)
	binary.BigEndian.PutUint64(dataFile[72:], qcow2IncompatDataFile)
	backing := qcow2Fixture(4096)
	binary.BigEndian.PutUint64(backing[8:], 512) // backing_file_offset
	binary.BigEndian.PutUint32(backing[16:], 11) // backing_file_size

	for _, tt := range []struct {
		name, file string
		data       []byte
		want       VolumeFormat
		wantErr    string
	}{
		{"raw without an extension", "disk.bin", bootableRaw(1024), VolumeFormatRaw, ""},
		{"raw named .qcow2", "disk.qcow2", bootableRaw(1024), VolumeFormatRaw, ""},
		{"qcow2 named .img", "disk.img", qcow2Fixture(4096), VolumeFormatQCOW2, ""},
		{"qcow2 with an external data file", "disk.qcow2", dataFile, "", "external data file"},
		{"qcow2 with a backing file", "disk.qcow2", backing, "", "backing file"},
		{"not an image", "notes.txt", []byte(strings.Repeat("x", 1024)), "", "unsupported or invalid image"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(path, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := CheckBootImageFile(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if got != tt.want {
				t.Errorf("format %q, want %q", got, tt.want)
			}
		})
	}

	if _, err := CheckBootImageFile(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("directory: got %v, want a not-a-regular-file error", err)
	}
}

// TestGenerateVolumeXML_BackingFormat: a detected backing format is named
// in the overlay whatever the backing path's extension says.
func TestGenerateVolumeXML_BackingFormat(t *testing.T) {
	spec := VolumeSpec{Name: "vm_boot.qcow2", Type: VolumeTypeBoot, Format: VolumeFormatQCOW2, CapacityGB: 1,
		BackingVolume: "/srv/images/disk.qcow2", BackingFormat: VolumeFormatRaw}
	xml, err := generateVolumeXML("pool", spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, `<format type="raw">`) || !strings.Contains(xml, "<path>/srv/images/disk.qcow2</path>") {
		t.Errorf("backing store does not name the detected raw format:\n%s", xml)
	}
}
