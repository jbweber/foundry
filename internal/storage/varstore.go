package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/jbweber/foundry/internal/naming"
)

// MaxVarstoreSize bounds an imported varstore. x86_64 OVMF varstores are
// 528 KiB and aarch64 AAVMF ones 64 MiB (the size of the virt machine's
// second flash device).
const MaxVarstoreSize = 64 << 20

// systemNvDataFvGUID is the file system GUID of an edk2 variable store
// firmware volume, gEfiSystemNvDataFvGuid {FFF12B8D-7696-4C8B-A985-2747075B4F50}
// (https://github.com/tianocore/edk2/blob/master/MdeModulePkg/MdeModulePkg.dec),
// in the byte order it has on disk.
var systemNvDataFvGUID = []byte{
	0x8d, 0x2b, 0xf1, 0xff, 0x96, 0x76, 0x8b, 0x4c,
	0xa9, 0x85, 0x27, 0x47, 0x07, 0x5b, 0x4f, 0x50,
}

// ValidateVarstore checks that data is a raw edk2 varstore: it starts with an
// EFI_FIRMWARE_VOLUME_HEADER (ZeroVector[16], FileSystemGuid at 16, FvLength
// at 32, Signature "_FVH" at 40; UEFI PI specification, volume 3,
// https://uefi.org/specs/PI/1.8/V3_Code_Definitions.html#efi-firmware-volume-header)
// whose file system is the variable store, its size is a whole number of
// 4 KiB blocks, and the firmware volume fits in the file. QEMU maps the file
// as a flash device of exactly its size, so the bytes are kept as they are.
func ValidateVarstore(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("varstore is empty")
	}
	if len(data) > MaxVarstoreSize {
		return fmt.Errorf("varstore is %d bytes, more than the %d allowed", len(data), MaxVarstoreSize)
	}
	if len(data)%4096 != 0 {
		return fmt.Errorf("varstore is %d bytes, not a whole number of 4 KiB blocks", len(data))
	}
	if !bytes.Equal(data[40:44], []byte("_FVH")) {
		return fmt.Errorf("not a raw edk2 varstore: no firmware volume signature \"_FVH\" at offset 40 (a qcow2 varstore is not supported)")
	}
	if !bytes.Equal(data[16:32], systemNvDataFvGUID) {
		return fmt.Errorf("firmware volume is not a variable store (file system GUID is not gEfiSystemNvDataFvGuid)")
	}
	if fvLength := binary.LittleEndian.Uint64(data[32:40]); fvLength == 0 || fvLength > uint64(len(data)) {
		return fmt.Errorf("firmware volume length %d does not fit the %d-byte file", fvLength, len(data))
	}
	return nil
}

// ReadVarstore reads a varstore of at most MaxVarstoreSize bytes and
// validates it.
func ReadVarstore(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxVarstoreSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read varstore: %w", err)
	}
	if err := ValidateVarstore(data); err != nil {
		return nil, err
	}
	return data, nil
}

// VarstoreInfo describes an imported Secure Boot key profile varstore.
type VarstoreInfo struct {
	Profile string // Key profile name
	Arch    string // Guest architecture (libvirt's name)
	Volume  string // Volume name in the foundry-images pool
	Path    string // Host path libvirt reports for the volume
	Bytes   uint64 // Size in bytes
}

// ImportVarstore stores a key profile's varstore as a volume of the
// foundry-images pool, sized to the byte, through the libvirt API
// (StorageVolCreateXML, then StorageVolUpload). With replace, an existing
// volume of the same profile and architecture is deleted first; VMs created
// from it keep their own copy, which libvirt made on their first start.
func (m *Manager) ImportVarstore(ctx context.Context, profile, arch string, data []byte, replace bool) error {
	if err := ValidateVarstore(data); err != nil {
		return err
	}
	name := naming.VolumeNameVarstore(profile, arch)
	exists, err := m.VolumeExists(ctx, DefaultImagesPool, name)
	if err != nil {
		return err
	}
	if exists {
		if !replace {
			return fmt.Errorf("varstore %s already exists in %s (use --replace)", name, DefaultImagesPool)
		}
		if err := m.DeleteVolume(ctx, DefaultImagesPool, name); err != nil {
			return fmt.Errorf("failed to delete the old varstore: %w", err)
		}
	}
	spec := VolumeSpec{
		Name:          name,
		Type:          VolumeTypeVarstore,
		Format:        VolumeFormatRaw,
		CapacityBytes: uint64(len(data)),
	}
	if err := m.CreateVolume(ctx, DefaultImagesPool, spec); err != nil {
		return fmt.Errorf("failed to create varstore volume: %w", err)
	}
	if err := m.WriteVolumeData(ctx, DefaultImagesPool, name, data); err != nil {
		_ = m.DeleteVolume(ctx, DefaultImagesPool, name)
		return fmt.Errorf("failed to upload varstore: %w", err)
	}
	return nil
}

// ListVarstores lists the key profile varstores in the foundry-images pool,
// sorted by profile and architecture.
func (m *Manager) ListVarstores(ctx context.Context) ([]VarstoreInfo, error) {
	vols, err := m.ListVolumes(ctx, DefaultImagesPool)
	if err != nil {
		return nil, err
	}
	var out []VarstoreInfo
	for _, v := range vols {
		profile, arch, ok := naming.ParseVolumeNameVarstore(v.Name)
		if !ok {
			continue
		}
		out = append(out, VarstoreInfo{Profile: profile, Arch: arch, Volume: v.Name, Path: v.Path, Bytes: v.Capacity})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Profile != out[j].Profile {
			return out[i].Profile < out[j].Profile
		}
		return out[i].Arch < out[j].Arch
	})
	return out, nil
}

// DeleteVarstore deletes a key profile's varstore volume. VMs created from it
// are not affected: each has its own varstore.
func (m *Manager) DeleteVarstore(ctx context.Context, profile, arch string) error {
	name := naming.VolumeNameVarstore(profile, arch)
	exists, err := m.VolumeExists(ctx, DefaultImagesPool, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("key profile %s has no %s varstore in %s", profile, arch, DefaultImagesPool)
	}
	return m.DeleteVolume(ctx, DefaultImagesPool, name)
}
