package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Magic bytes and signatures for disk image format detection
var (
	// qcow2Magic is the magic bytes at the start of QCOW2 files: "QFI" + 0xfb
	// QCOW2 images begin with a file header where bytes 0-3 contain the magic
	// string 0x514649fb, which is ASCII "QFI" followed by 0xfb.
	// Reference: https://www.qemu.org/docs/master/interop/qcow2.html
	qcow2Magic = []byte{0x51, 0x46, 0x49, 0xfb}

	// mbrSignature is the boot sector signature at offset 510 in bootable disks.
	// This is the standard MBR boot signature 0x55 0xaa that appears at the end
	// of the first 512-byte sector on all bootable disks (MBR and GPT).
	// Reference: https://en.wikipedia.org/wiki/Master_boot_record
	mbrSignature = []byte{0x55, 0xaa}
)

// DetectImageFormat detects the disk image format by reading magic bytes.
// Returns VolumeFormatQCOW2 for QCOW2 images, or VolumeFormatRaw for bootable RAW images.
// Returns error if the format is unsupported or the file is not a valid bootable image.
//
// Validation rules:
//   - QCOW2: Must have magic bytes "QFI\xfb" (0x51 0x46 0x49 0xfb) at offset 0
//   - RAW: Must have MBR signature 0x55 0xaa at offset 510 (boot sector end)
//
// This ensures that imported images are valid bootable OS images, not arbitrary data files.
//
// Note: The MBR signature check works for both MBR and GPT partitioned disks. GPT disks
// include a "protective MBR" in the first sector that also contains the 0x55aa signature,
// as specified in UEFI Specification 2.10, Section 5 (GUID Partition Table Format).
// Reference: https://uefi.org/specs/UEFI/2.10/05_GUID_Partition_Table_Format.html
func DetectImageFormat(filePath string) (VolumeFormat, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Read first 4 bytes to check for QCOW2 magic
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return "", fmt.Errorf("file too small to be valid image (< 4 bytes): %w", err)
	}

	// Check for QCOW2 magic
	if bytes.Equal(magic, qcow2Magic) {
		return VolumeFormatQCOW2, nil
	}

	// Not QCOW2, check if it's a bootable RAW image
	// MBR signature: 0x55 0xaa at offset 510 (end of first 512-byte boot sector)
	if _, err := f.Seek(510, 0); err != nil {
		return "", fmt.Errorf("failed to seek to boot sector signature: %w", err)
	}

	sig := make([]byte, 2)
	if _, err := io.ReadFull(f, sig); err != nil {
		return "", fmt.Errorf("file too small for boot sector (< 512 bytes): %w", err)
	}

	if bytes.Equal(sig, mbrSignature) {
		return VolumeFormatRaw, nil
	}

	return "", fmt.Errorf("unsupported or invalid image: not qcow2 and missing boot sector signature (0x55aa at offset 510)")
}
