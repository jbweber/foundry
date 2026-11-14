package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	return detectImageFormat(f)
}

// detectImageFormat is DetectImageFormat on content already open.
func detectImageFormat(f io.ReadSeeker) (VolumeFormat, error) {
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
	if _, err := f.Seek(510, io.SeekStart); err != nil {
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

// Fields of the qcow2 header and its extensions that make qemu read another
// file. No Go library for the format is among foundry's dependencies, and
// foundry reaches the host through libvirt only (no qemu-img), so these few
// documented fields are read here. All of them are big-endian on every host.
// Reference: https://www.qemu.org/docs/master/interop/qcow2.html
const (
	qcow2HeaderV2Len = 72  // version 2's header; version 3's says its own length
	qcow2HeaderV3Len = 104 // the shortest header version 3 allows

	// Incompatible feature bits the specification defines: dirty (0), corrupt
	// (1), external data file (2), compression type (3), extended L2 (4).
	qcow2IncompatDataFile = 1 << 2
	qcow2IncompatKnown    = 1<<5 - 1

	qcow2ExtEnd           = 0x00000000 // the end of the header extensions
	qcow2ExtBackingFormat = 0xe2792aca
	qcow2ExtDataFile      = 0x44415441

	// qcow2HeadBytes is how much of a qcow2 image the check reads: its
	// header extensions must end within it. qemu reads them up to the end of
	// the image's first cluster; 64 KiB is qemu-img's default cluster size.
	qcow2HeadBytes = 64 * 1024
)

// checkQCOW2Header refuses a qcow2 image that would make qemu read another
// file: a VM booted from it would get that file's content, any file qemu may
// open on the host. head is the start of the image (up to qcow2HeadBytes).
// It refuses:
//   - a backing file (backing_file_offset, backing_file_size, or a backing
//     format extension);
//   - an external data file (incompatible feature bit 2, or its extension);
//   - what it cannot vouch for: a version other than 2 or 3, an incompatible
//     feature the specification does not define (qemu refuses those too), and
//     header extensions that do not end within head.
func checkQCOW2Header(head []byte) error {
	if len(head) < qcow2HeaderV2Len {
		return fmt.Errorf("qcow2 header too short (%d bytes)", len(head))
	}
	be := binary.BigEndian
	version := be.Uint32(head[4:])
	if version != 2 && version != 3 {
		return fmt.Errorf("qcow2 version %d is not accepted (only 2 and 3)", version)
	}
	if be.Uint64(head[8:]) != 0 || be.Uint32(head[16:]) != 0 {
		return errors.New("qcow2 image has a backing file: not accepted, as qemu would read that file from the host")
	}
	extensions := uint64(qcow2HeaderV2Len)
	if version == 3 {
		if len(head) < qcow2HeaderV3Len {
			return fmt.Errorf("qcow2 header too short (%d bytes)", len(head))
		}
		features := be.Uint64(head[72:])
		if features&qcow2IncompatDataFile != 0 {
			return errors.New("qcow2 image has an external data file: not accepted, as qemu would read that file from the host")
		}
		if features&^qcow2IncompatKnown != 0 {
			return fmt.Errorf("qcow2 image has unknown incompatible features (%#x)", features&^qcow2IncompatKnown)
		}
		extensions = uint64(be.Uint32(head[100:]))
		if extensions < qcow2HeaderV3Len {
			return fmt.Errorf("qcow2 header_length %d is too short", extensions)
		}
	}
	// Each extension: type, length (4 bytes each), data padded to 8 bytes.
	// Offsets are uint64, so no field value can wrap them on any platform.
	for off := extensions; ; {
		if off+8 > uint64(len(head)) {
			return errors.New("qcow2 header extensions do not end within the part of the image that is checked")
		}
		extType, extLen := be.Uint32(head[off:]), uint64(be.Uint32(head[off+4:]))
		switch extType {
		case qcow2ExtEnd:
			return nil
		case qcow2ExtBackingFormat:
			return errors.New("qcow2 image has a backing format: not accepted, as it goes with a backing file")
		case qcow2ExtDataFile:
			return errors.New("qcow2 image names an external data file: not accepted, as qemu would read that file from the host")
		}
		off += 8 + (extLen+7)&^7
	}
}

// CheckBootImageFile checks a boot image given as a host path rather than an
// imported image, as import checks an image: it must be a regular file whose
// content is a qcow2 or a bootable raw image, and a qcow2 must not make qemu
// read another file (see checkQCOW2Header). It returns the format detected
// from the content, never from the file's name, for the overlay to name as
// its backing format, and the disk size a guest booted from it sees: a
// qcow2's virtual size, a raw image's length. The path is read on the
// machine foundry runs on, which is the hypervisor unless libvirt is reached
// over --socket.
func CheckBootImageFile(path string) (VolumeFormat, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open boot image: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat boot image: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("boot image %s is not a regular file", path)
	}
	format, err := detectImageFormat(f)
	if err != nil {
		return "", 0, fmt.Errorf("boot image %s: %w", path, err)
	}
	if format == VolumeFormatQCOW2 {
		head := make([]byte, min(info.Size(), qcow2HeadBytes))
		if _, err := f.ReadAt(head, 0); err != nil {
			return "", 0, fmt.Errorf("failed to read the qcow2 header of boot image %s: %w", path, err)
		}
		if err := checkQCOW2Header(head); err != nil {
			return "", 0, fmt.Errorf("boot image %s: %w", path, err)
		}
		// The header's size field (bytes 24-31): the virtual disk size
		return format, binary.BigEndian.Uint64(head[24:]), nil
	}
	return format, uint64(info.Size()), nil
}
