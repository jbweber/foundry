package storage

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qcow2Fixture is a version 3 qcow2 header with no backing file, no
// extensions (the end marker right after the 104-byte header), padded to size
// bytes.
func qcow2Fixture(size int) []byte {
	b := make([]byte, size)
	copy(b, qcow2Magic)
	binary.BigEndian.PutUint32(b[4:], 3)                  // version
	binary.BigEndian.PutUint32(b[20:], 16)                // cluster_bits
	binary.BigEndian.PutUint32(b[100:], qcow2HeaderV3Len) // header_length
	return b
}

// qcow2Ext appends a header extension at off and returns the offset after it.
func qcow2Ext(b []byte, off int, typ uint32, data []byte) int {
	binary.BigEndian.PutUint32(b[off:], typ)
	binary.BigEndian.PutUint32(b[off+4:], uint32(len(data)))
	copy(b[off+8:], data)
	return off + 8 + (len(data)+7)&^7
}

func TestCheckQCOW2Header(t *testing.T) {
	be := binary.BigEndian
	for _, tt := range []struct {
		name    string
		head    func() []byte
		wantErr string // empty: accepted
	}{
		{"version 3, no extensions", func() []byte { return qcow2Fixture(512) }, ""},
		{"version 2", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[4:], 2)
			be.PutUint32(b[100:], 0) // inside version 2's extension area: the end marker
			return b
		}, ""},
		{"dirty and corrupt bits", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[72:], 0b11)
			return b
		}, ""},
		{"compression type and extended L2 bits", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[72:], 0b11000)
			return b
		}, ""},
		{"harmless extensions before the end", func() []byte {
			b := qcow2Fixture(512)
			off := qcow2Ext(b, qcow2HeaderV3Len, 0x6803f857, make([]byte, 48)) // feature name table
			qcow2Ext(b, off, 0x23852875, make([]byte, 5))                      // bitmaps, padded
			return b
		}, ""},
		{"longer header_length", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[100:], 112)
			return b
		}, ""},
		{"backing file offset", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[8:], 300)
			return b
		}, "backing file"},
		{"backing file size", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[16:], 10)
			return b
		}, "backing file"},
		{"version 2 backing file", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[4:], 2)
			be.PutUint32(b[100:], 0)
			be.PutUint64(b[8:], 72)
			be.PutUint32(b[16:], 9)
			return b
		}, "backing file"},
		{"backing format extension", func() []byte {
			b := qcow2Fixture(512)
			qcow2Ext(b, qcow2HeaderV3Len, qcow2ExtBackingFormat, []byte("raw"))
			return b
		}, "backing format"},
		{"external data file bit", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[72:], qcow2IncompatDataFile)
			return b
		}, "external data file"},
		{"external data file extension", func() []byte {
			b := qcow2Fixture(512)
			off := qcow2Ext(b, qcow2HeaderV3Len, 0x6803f857, make([]byte, 48))
			qcow2Ext(b, off, qcow2ExtDataFile, []byte("/etc/shadow"))
			return b
		}, "external data file"},
		{"unknown incompatible feature", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[72:], 1<<5)
			return b
		}, "unknown incompatible features"},
		{"highest incompatible feature bit", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint64(b[72:], 1<<63)
			return b
		}, "unknown incompatible features"},
		{"version 1", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[4:], 1)
			return b
		}, "version 1"},
		{"version 4", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[4:], 4)
			return b
		}, "version 4"},
		{"header_length too short", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[100:], 72)
			return b
		}, "header_length"},
		{"header_length past the checked part", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[100:], 0xffffffff)
			return b
		}, "do not end"},
		{"no end marker", func() []byte {
			b := qcow2Fixture(128)
			qcow2Ext(b, qcow2HeaderV3Len, 0x6803f857, make([]byte, 16))
			return b
		}, "do not end"},
		{"extension length past the checked part", func() []byte {
			b := qcow2Fixture(512)
			be.PutUint32(b[qcow2HeaderV3Len:], 0x6803f857)
			be.PutUint32(b[qcow2HeaderV3Len+4:], 0xffffffff)
			return b
		}, "do not end"},
		{"version 2 too short", func() []byte { return qcow2Fixture(512)[:71] }, "too short"},
		{"version 3 too short", func() []byte { return qcow2Fixture(512)[:100] }, "too short"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkQCOW2Header(tt.head())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// The header comes from an untrusted image: no content may make the check
// panic or loop.
func FuzzCheckQCOW2Header(f *testing.F) {
	f.Add(qcow2Fixture(512))
	b := qcow2Fixture(256)
	qcow2Ext(b, qcow2HeaderV3Len, 0x6803f857, make([]byte, 16))
	f.Add(b)
	f.Fuzz(func(t *testing.T, head []byte) {
		_ = checkQCOW2Header(head)
	})
}

// An import refuses a qcow2 that makes qemu read another file, and makes no
// volume for it.
func TestManager_ImportImage_RefusesQCOW2ReadingHostFiles(t *testing.T) {
	for _, tt := range []struct {
		name    string
		edit    func(b []byte)
		wantErr string
	}{
		{"backing file", func(b []byte) { binary.BigEndian.PutUint64(b[8:], 300) }, "backing file"},
		{"external data file", func(b []byte) { binary.BigEndian.PutUint64(b[72:], qcow2IncompatDataFile) }, "external data file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := qcow2Fixture(4096)
			tt.edit(data)
			path := filepath.Join(t.TempDir(), "x.qcow2")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			client, mgr := newImagesManager(t)
			err := mgr.ImportImage(context.Background(), path, "x.qcow2")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			if len(client.volumes[DefaultImagesPool]) != 0 {
				t.Error("a volume was created for a refused image")
			}
		})
	}
}
