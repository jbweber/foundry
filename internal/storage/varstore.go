package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

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

// archAarch64 is libvirt's name for the one architecture with JSON varstores.
const archAarch64 = "aarch64"

// The variables that make a JSON varstore enforce Secure Boot, and their
// namespaces: EFI_GLOBAL_VARIABLE and EFI_SECURE_BOOT_ENABLE_DISABLE, as in
// https://gitlab.com/qemu-project/qemu/-/blob/v11.1.0/hw/uefi/var-service-auth.c
const (
	efiGlobalVariableGUID = "8be4df61-93ca-11d2-aa0d-00e098032b8c"
	secureBootEnableGUID  = "f0a30bc7-af08-4556-99c4-001009c93a44"
)

var varstoreGUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// jsonVarstore is the file format of QEMU's uefi-vars device, UefiVarStore in
// https://gitlab.com/qemu-project/qemu/-/blob/v11.1.0/qapi/uefi.json
type jsonVarstore struct {
	Version   *int `json:"version"`
	Variables []struct {
		GUID   string `json:"guid"`
		Name   string `json:"name"`
		Attr   *int64 `json:"attr"`
		Data   string `json:"data"`
		Time   string `json:"time"`
		Digest string `json:"digest"`
	} `json:"variables"`
}

// isJSONVarstore tells the two forms apart: a raw varstore starts with the
// zero vector of its firmware volume header, a JSON one with an object.
func isJSONVarstore(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("{"))
}

// ValidateVarstoreJSON checks that data is a JSON varstore QEMU's uefi-vars
// device can load, as strictly as QEMU's own parser takes it (a field it does
// not know fails the VM's start), and that it is a Secure Boot key profile: a
// PK is set and SecureBootEnable is on. Without those the store is valid and
// the guest boots, but in Setup Mode, where nothing is enforced and the guest
// can enrol keys of its own (setup_mode_is_active in QEMU's var-service-auth.c).
func ValidateVarstoreJSON(data []byte) error {
	if len(data) > MaxVarstoreSize {
		return fmt.Errorf("varstore is %d bytes, more than the %d allowed", len(data), MaxVarstoreSize)
	}
	var store jsonVarstore
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&store); err != nil {
		return fmt.Errorf("not a JSON varstore: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("not a JSON varstore: more follows the object")
	}
	if store.Version == nil || *store.Version != 2 {
		return fmt.Errorf("JSON varstore: version must be 2")
	}
	var hasPK, secureBootOn bool
	for i, v := range store.Variables {
		if v.Name == "" || v.Attr == nil {
			return fmt.Errorf("JSON varstore: variable %d needs a name and attr", i)
		}
		if !varstoreGUID.MatchString(v.GUID) {
			return fmt.Errorf("JSON varstore: variable %s: guid %q is not a GUID", v.Name, v.GUID)
		}
		for field, value := range map[string]string{"data": v.Data, "time": v.Time, "digest": v.Digest} {
			if _, err := hex.DecodeString(value); err != nil {
				return fmt.Errorf("JSON varstore: variable %s: %s is not hex: %w", v.Name, field, err)
			}
		}
		guid := strings.ToLower(v.GUID)
		hasPK = hasPK || (v.Name == "PK" && guid == efiGlobalVariableGUID && v.Data != "")
		secureBootOn = secureBootOn || (v.Name == "SecureBootEnable" && guid == secureBootEnableGUID && v.Data == "01")
	}
	if !hasPK || !secureBootOn {
		return fmt.Errorf("JSON varstore: no PK, or SecureBootEnable is not on, so the guest would not enforce Secure Boot (make it with virt-fw-vars --set-pk ... --secure-boot)")
	}
	return nil
}

// ValidateVarstoreFor checks data as a varstore for a guest architecture:
// aarch64 takes either form, every other architecture the raw form only.
func ValidateVarstoreFor(arch string, data []byte) error {
	if arch == archAarch64 && isJSONVarstore(data) {
		return ValidateVarstoreJSON(data)
	}
	return ValidateVarstore(data)
}

// ReadVarstore reads a varstore of at most MaxVarstoreSize bytes, of either
// form, and validates it. Whether its form suits the architecture is
// ImportVarstore's to check, which knows the architecture.
func ReadVarstore(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxVarstoreSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read varstore: %w", err)
	}
	if isJSONVarstore(data) {
		err = ValidateVarstoreJSON(data)
	} else {
		err = ValidateVarstore(data)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// varstoreVolumes returns the volume names a profile can have for an
// architecture: one of these is the profile's, whatever its form.
func varstoreVolumes(profile, arch string) []string {
	names := []string{naming.VolumeNameVarstore(profile, arch)}
	if arch == archAarch64 {
		names = append(names, naming.VolumeNameVarstoreJSON(profile, arch))
	}
	return names
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
// (StorageVolCreateXML, then StorageVolUpload), named for its form (raw .fd,
// or for aarch64 a uefi-vars .json). A profile has one form per architecture.
// With replace, an existing volume of the same form is read back, deleted,
// and put back as it was if the new one cannot be written, so a failed
// replace leaves the old profile in place; one of the other form is deleted
// only once the new one is written. VMs created from it keep their own copy,
// which libvirt made on their first start.
func (m *Manager) ImportVarstore(ctx context.Context, profile, arch string, data []byte, replace bool) error {
	if err := ValidateVarstoreFor(arch, data); err != nil {
		return err
	}
	name := naming.VolumeNameVarstore(profile, arch)
	if isJSONVarstore(data) {
		name = naming.VolumeNameVarstoreJSON(profile, arch)
	}
	var others []string
	var old []byte
	for _, vol := range varstoreVolumes(profile, arch) {
		exists, err := m.VolumeExists(ctx, DefaultImagesPool, vol)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if !replace {
			return fmt.Errorf("varstore %s already exists in %s (use --replace)", vol, DefaultImagesPool)
		}
		if vol != name {
			others = append(others, vol)
			continue
		}
		// libvirt has no rename, so the new profile can't be written
		// beside the old one under its name: keep the old one's content
		// (a varstore is small) to put it back if writing the new fails
		if old, err = m.readVolume(DefaultImagesPool, name, MaxVarstoreSize); err != nil {
			return fmt.Errorf("failed to read the old varstore, nothing changed: %w", err)
		}
		if err := m.DeleteVolume(ctx, DefaultImagesPool, name); err != nil {
			return fmt.Errorf("failed to delete the old varstore: %w", err)
		}
	}
	if err := m.writeVarstore(ctx, name, data); err != nil {
		if old == nil {
			return err
		}
		if rerr := m.writeVarstore(ctx, name, old); rerr != nil {
			return fmt.Errorf("%w; the old varstore could not be put back either, import it again: %v", err, rerr)
		}
		return fmt.Errorf("%w; the old varstore was put back", err)
	}
	// The profile now has its new form; the other form's volume goes only
	// after it is written, so a failure leaves the old profile in place.
	for _, vol := range others {
		if err := m.DeleteVolume(ctx, DefaultImagesPool, vol); err != nil {
			return fmt.Errorf("the new varstore %s is stored, but the old %s could not be deleted, so the profile has both: %w", name, vol, err)
		}
	}
	return nil
}

// writeVarstore creates the varstore volume name, sized to the byte, and
// uploads data into it; on a failed upload it deletes the volume again.
func (m *Manager) writeVarstore(ctx context.Context, name string, data []byte) error {
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
		_ = m.DeleteVolume(context.WithoutCancel(ctx), DefaultImagesPool, name)
		return fmt.Errorf("failed to upload varstore: %w", err)
	}
	return nil
}

// readVolume reads a volume's content through libvirt (StorageVolDownload),
// refusing one longer than limit bytes.
func (m *Manager) readVolume(poolName, volumeName string, limit int) ([]byte, error) {
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("pool not found: %w", err)
	}
	vol, err := m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %w", err)
	}
	buf := limitedBuffer{limit: limit}
	if err := m.client.StorageVolDownload(vol, &buf, 0, 0, 0); err != nil {
		return nil, fmt.Errorf("failed to read volume %s/%s: %w", poolName, volumeName, err)
	}
	return buf.Bytes(), nil
}

// limitedBuffer is a bytes.Buffer that refuses to grow past limit bytes.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("volume is larger than %d bytes", b.limit)
	}
	return b.Buffer.Write(p)
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

// DeleteVarstore deletes a key profile's varstore volume for an
// architecture, of either form. VMs created from it are not affected: each
// has its own varstore.
func (m *Manager) DeleteVarstore(ctx context.Context, profile, arch string) error {
	deleted := false
	for _, name := range varstoreVolumes(profile, arch) {
		exists, err := m.VolumeExists(ctx, DefaultImagesPool, name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := m.DeleteVolume(ctx, DefaultImagesPool, name); err != nil {
			return err
		}
		deleted = true
	}
	if !deleted {
		return fmt.Errorf("key profile %s has no %s varstore in %s", profile, arch, DefaultImagesPool)
	}
	return nil
}
