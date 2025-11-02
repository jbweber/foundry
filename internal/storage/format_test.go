package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectImageFormat(t *testing.T) {
	tests := []struct {
		name       string
		setupFile  func(string) error
		wantFormat VolumeFormat
		wantErr    bool
	}{
		{
			name: "qcow2 image with valid magic",
			setupFile: func(path string) error {
				// Write QCOW2 magic + minimal header (at least 512 bytes for full test)
				data := []byte{0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x03} // Magic + version 3
				data = append(data, make([]byte, 504)...)                      // Pad to 512 bytes
				return os.WriteFile(path, data, 0644)
			},
			wantFormat: VolumeFormatQCOW2,
			wantErr:    false,
		},
		{
			name: "bootable raw image with MBR signature",
			setupFile: func(path string) error {
				// Create 512-byte boot sector with MBR signature at offset 510-511
				data := make([]byte, 512)
				data[510] = 0x55
				data[511] = 0xaa
				return os.WriteFile(path, data, 0644)
			},
			wantFormat: VolumeFormatRaw,
			wantErr:    false,
		},
		{
			name: "bootable raw image larger than 512 bytes",
			setupFile: func(path string) error {
				// Create larger image (simulating real disk)
				data := make([]byte, 4096) // 4KB
				data[510] = 0x55
				data[511] = 0xaa
				return os.WriteFile(path, data, 0644)
			},
			wantFormat: VolumeFormatRaw,
			wantErr:    false,
		},
		{
			name: "non-bootable raw image without MBR signature",
			setupFile: func(path string) error {
				// Just zeros, no boot signature
				return os.WriteFile(path, make([]byte, 512), 0644)
			},
			wantFormat: "",
			wantErr:    true, // Should reject non-bootable
		},
		{
			name: "non-bootable raw with wrong signature bytes",
			setupFile: func(path string) error {
				// Wrong signature at offset 510
				data := make([]byte, 512)
				data[510] = 0xaa // Reversed!
				data[511] = 0x55
				return os.WriteFile(path, data, 0644)
			},
			wantFormat: "",
			wantErr:    true,
		},
		{
			name: "file too small (< 4 bytes)",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte{0x01, 0x02}, 0644)
			},
			wantFormat: "",
			wantErr:    true,
		},
		{
			name: "file too small for boot sector (< 512 bytes)",
			setupFile: func(path string) error {
				// Not QCOW2, but too small for boot sector check
				return os.WriteFile(path, make([]byte, 256), 0644)
			},
			wantFormat: "",
			wantErr:    true,
		},
		{
			name: "empty file",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte{}, 0644)
			},
			wantFormat: "",
			wantErr:    true,
		},
		{
			name:       "non-existent file",
			setupFile:  func(path string) error { return nil }, // Don't create file
			wantFormat: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test-image")

			if err := tt.setupFile(filePath); err != nil {
				t.Fatalf("Failed to setup test file: %v", err)
			}

			format, err := DetectImageFormat(filePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectImageFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if format != tt.wantFormat {
				t.Errorf("DetectImageFormat() = %v, want %v", format, tt.wantFormat)
			}
		})
	}
}

func TestDetectImageFormat_RealQCOW2(t *testing.T) {
	// Test with a more realistic QCOW2 header structure
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "realistic.qcow2")

	// QCOW2 header structure (minimal valid header)
	header := []byte{
		0x51, 0x46, 0x49, 0xfb, // Magic: "QFI\xfb"
		0x00, 0x00, 0x00, 0x03, // Version: 3
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Backing file offset: 0
		0x00, 0x00, 0x00, 0x04, // Backing file size: 4
		0x00, 0x00, 0x00, 0x10, // Cluster bits: 16 (64KB clusters)
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, // Size: 16MB
	}
	// Pad to at least 512 bytes
	header = append(header, make([]byte, 512-len(header))...)

	if err := os.WriteFile(filePath, header, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	format, err := DetectImageFormat(filePath)
	if err != nil {
		t.Errorf("DetectImageFormat() unexpected error: %v", err)
	}

	if format != VolumeFormatQCOW2 {
		t.Errorf("DetectImageFormat() = %v, want %v", format, VolumeFormatQCOW2)
	}
}

func TestDetectImageFormat_RealMBR(t *testing.T) {
	// Test with a more realistic MBR boot sector structure
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "realistic-mbr.raw")

	// Create a minimal but realistic MBR structure
	data := make([]byte, 512)

	// Bootstrap code area (0-445) - just use zeros for test
	// Disk signature at offset 440-443
	data[440] = 0x12
	data[441] = 0x34
	data[442] = 0x56
	data[443] = 0x78

	// Partition entry 1 (offset 446-461)
	data[446] = 0x80                         // Bootable flag
	data[450] = 0x83                         // Partition type (Linux)
	copy(data[454:458], []byte{0, 8, 0, 0})  // Starting LBA
	copy(data[458:462], []byte{0, 0, 16, 0}) // Size in sectors

	// Boot signature at offset 510-511
	data[510] = 0x55
	data[511] = 0xaa

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	format, err := DetectImageFormat(filePath)
	if err != nil {
		t.Errorf("DetectImageFormat() unexpected error: %v", err)
	}

	if format != VolumeFormatRaw {
		t.Errorf("DetectImageFormat() = %v, want %v", format, VolumeFormatRaw)
	}
}
