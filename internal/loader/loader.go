// Package loader provides functions for loading VirtualMachine resources
// from YAML files.
package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/naming"
)

// imagesPool is the pool create takes a boot disk's image volume from.
const imagesPool = "foundry-images"

// MaxVlanID is the highest assignable 802.1Q VLAN ID. The 12-bit VLAN field
// spans 0-4095, but 0 denotes a priority-tagged frame and 4095 is reserved,
// so the usable range is 1-4094.
const MaxVlanID = 4094

// MaxVlanTrunk is the most IDs one trunk interface may list. libvirt adds each
// ID to the bridge port one at a time, so a short list keeps a start quick;
// raise it when a caller needs more.
const MaxVlanTrunk = 256

// LoadFromFile loads a VirtualMachine resource from a YAML file.
// The file must be in the foundry.cofront.xyz/v1alpha1 format.
func LoadFromFile(path string) (*v1alpha1.VirtualMachine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	vm, err := LoadFromYAML(data)
	if err != nil {
		return nil, err
	}
	resolveInputFiles(vm, filepath.Dir(path))
	return vm, nil
}

// resolveInputFiles makes relative spec.inputDisks[].files[].file paths
// absolute, from dir (the directory of the VM's YAML file), so they name the
// same files whatever directory foundry runs from, and the spec stored in the
// domain's metadata does too.
func resolveInputFiles(vm *v1alpha1.VirtualMachine, dir string) {
	for i := range vm.Spec.InputDisks {
		for j := range vm.Spec.InputDisks[i].Files {
			f := &vm.Spec.InputDisks[i].Files[j]
			if f.File == "" || filepath.IsAbs(f.File) {
				continue
			}
			if abs, err := filepath.Abs(filepath.Join(dir, f.File)); err == nil {
				f.File = abs
			}
		}
	}
}

// LoadFromYAML loads a VirtualMachine resource from YAML bytes.
// The YAML must be in the foundry.cofront.xyz/v1alpha1 format.
func LoadFromYAML(data []byte) (*v1alpha1.VirtualMachine, error) {
	var vm v1alpha1.VirtualMachine
	if err := yaml.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// Validate that apiVersion and kind are present
	if vm.APIVersion == "" {
		return nil, fmt.Errorf("missing required field: apiVersion")
	}
	if vm.Kind == "" {
		return nil, fmt.Errorf("missing required field: kind")
	}

	// Validate apiVersion matches expected
	expectedAPIVersion := v1alpha1.GroupName + "/" + v1alpha1.Version
	if vm.APIVersion != expectedAPIVersion {
		return nil, fmt.Errorf("unsupported apiVersion: %s (expected: %s)", vm.APIVersion, expectedAPIVersion)
	}

	// Validate kind
	if vm.Kind != v1alpha1.VirtualMachineKind {
		return nil, fmt.Errorf("unsupported kind: %s (expected: %s)", vm.Kind, v1alpha1.VirtualMachineKind)
	}

	// Set defaults for fields that may be omitted
	applyDefaults(&vm)

	// Validate the spec
	if err := validateSpec(&vm); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &vm, nil
}

// SaveToFile saves a VirtualMachine resource to a YAML file.
func SaveToFile(vm *v1alpha1.VirtualMachine, path string) error {
	// Ensure TypeMeta is set
	v1alpha1.SetDefaultAPIVersion(vm)

	data, err := v1alpha1.EncodeYAML(vm)
	if err != nil {
		return fmt.Errorf("failed to marshal VM to YAML: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// applyDefaults sets default values for optional fields.
func applyDefaults(vm *v1alpha1.VirtualMachine) {
	// Apply defaults from helpers. With spec.arch set, the CPU mode default
	// depends on the hypervisor's architecture (KVM or emulation), so it is
	// picked when the domain XML is generated.
	if vm.Spec.CPUMode == "" && vm.Spec.Arch == "" {
		vm.Spec.CPUMode = v1alpha1.CPUModeHostModel
	}
	if vm.Spec.StoragePool == "" {
		vm.Spec.StoragePool = "foundry-vms"
	}
	if vm.Spec.BootDisk.Format == "" {
		vm.Spec.BootDisk.Format = "qcow2"
	}
	if vm.Spec.BootDisk.ImagePool == "" {
		vm.Spec.BootDisk.ImagePool = imagesPool
	}
	if vm.Spec.Autostart == nil {
		autostart := true
		vm.Spec.Autostart = &autostart
	}

	// Set initial phase if not set
	if vm.Status.Phase == "" {
		vm.Status.Phase = v1alpha1.VMPhasePending
	}

	// Normalize name to lowercase
	vm.Name = strings.ToLower(vm.Name)

	// Normalize FQDN to lowercase
	if vm.Spec.CloudInit != nil && vm.Spec.CloudInit.FQDN != "" {
		vm.Spec.CloudInit.FQDN = strings.ToLower(vm.Spec.CloudInit.FQDN)
	}

	// Normalize the SMBIOS UUID to lowercase, the form libvirt reports a domain UUID in
	if vm.Spec.SMBIOS != nil {
		vm.Spec.SMBIOS.UUID = strings.ToLower(vm.Spec.SMBIOS.UUID)
	}
}

// vmNameRE is the form of a new VM's name. The name is the prefix of every
// volume name the VM gets ("<name>_boot.qcow2", "<name>_data-vdb.qcow2", ...),
// and destroy finds a VM's volumes by that prefix, so the name has no "_":
// otherwise VM "a" with data disk "b_boot" would name VM "a_data-b"'s boot
// volume, and "<name>_" would match the volumes of another VM. It has no "/"
// and no leading ".", so it can't name another directory in the pool's path.
// Only specs loaded for create (and render) are checked; existing VMs are
// listed, shown and destroyed by their domain name, whatever it is.
var vmNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)

// dataDeviceRE is the form of a data disk's device: a virtio disk target
// after vda (the boot disk), as libvirt names them. The device becomes part of
// the volume name, so this also keeps "_" and "/" out of it.
var dataDeviceRE = regexp.MustCompile(`^vd[b-z]{1,2}$`)

// validateSpec validates the VirtualMachine spec for required fields and consistency.
func validateSpec(vm *v1alpha1.VirtualMachine) error {
	// Validate metadata.name
	if vm.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if !vmNameRE.MatchString(vm.Name) {
		return fmt.Errorf("metadata.name %q must be 1 to 63 characters from a-z 0-9 . -, starting with a letter or digit", vm.Name)
	}

	// Validate VCPUs
	if vm.Spec.VCPUs <= 0 {
		return fmt.Errorf("spec.vcpus must be greater than 0")
	}

	// Validate memory
	if vm.Spec.MemoryGiB <= 0 {
		return fmt.Errorf("spec.memoryGiB must be greater than 0")
	}

	// Validate architecture
	switch vm.Spec.Arch {
	case "", v1alpha1.ArchX86_64, v1alpha1.ArchAarch64:
	default:
		return fmt.Errorf("spec.arch %q is not supported (expected %s or %s)", vm.Spec.Arch, v1alpha1.ArchX86_64, v1alpha1.ArchAarch64)
	}

	// QEMU implements host-model only for x86 (and ppc64), so libvirt refuses
	// it for an Arm guest at start time; fail early instead.
	if vm.GetArch() == v1alpha1.ArchAarch64 && vm.Spec.CPUMode == v1alpha1.CPUModeHostModel {
		return fmt.Errorf("spec.cpuMode host-model is not supported for aarch64; use host-passthrough (kvm) or maximum (emulation)")
	}

	if err := validateFirmware(vm); err != nil {
		return err
	}
	if err := validateSMBIOS(vm); err != nil {
		return err
	}
	if err := validateFwCfg("spec.fwCfg", vm.Spec.FwCfg); err != nil {
		return err
	}
	if err := validateInputDisks(vm); err != nil {
		return err
	}

	// Validate boot disk
	if vm.Spec.BootDisk.SizeGB <= 0 {
		return fmt.Errorf("spec.bootDisk.sizeGB must be greater than 0")
	}

	// Boot disk must have either image or empty=true
	if vm.Spec.BootDisk.Image == "" && !vm.Spec.BootDisk.Empty {
		return fmt.Errorf("spec.bootDisk must specify either 'image' or 'empty: true'")
	}
	if vm.Spec.BootDisk.Image != "" && vm.Spec.BootDisk.Empty {
		return fmt.Errorf("spec.bootDisk cannot specify both 'image' and 'empty: true'")
	}
	if err := validateImageSHA256(vm.Spec.BootDisk); err != nil {
		return err
	}

	// Validate data disks
	devicesSeen := make(map[string]bool)
	for i, disk := range vm.Spec.DataDisks {
		if disk.Device == "" {
			return fmt.Errorf("spec.dataDisks[%d].device is required", i)
		}
		if !dataDeviceRE.MatchString(disk.Device) {
			return fmt.Errorf("spec.dataDisks[%d].device %q must be a virtio disk name from vdb to vdzz (vd and one or two of b-z); vda is the boot disk", i, disk.Device)
		}
		if disk.SizeGB <= 0 {
			return fmt.Errorf("spec.dataDisks[%d].sizeGB must be greater than 0", i)
		}
		if devicesSeen[disk.Device] {
			return fmt.Errorf("spec.dataDisks[%d].device %q is duplicated", i, disk.Device)
		}
		devicesSeen[disk.Device] = true
	}

	// Validate disk serials (boot disk and data disks share one namespace)
	if err := validateDiskSerials(vm); err != nil {
		return err
	}

	// Validate network interfaces
	if len(vm.Spec.NetworkInterfaces) == 0 {
		return fmt.Errorf("spec.networkInterfaces must have at least one interface")
	}

	if err := validateHostDevices(vm); err != nil {
		return err
	}

	ipsSeen := make(map[string]bool)
	defaultRouteIdx := -1
	pxeBootIdx := -1
	for i, iface := range vm.Spec.NetworkInterfaces {
		if iface.IP == "" {
			return fmt.Errorf("spec.networkInterfaces[%d].ip is required", i)
		}
		if iface.Gateway == "" {
			return fmt.Errorf("spec.networkInterfaces[%d].gateway is required", i)
		}
		if iface.Bridge == "" {
			return fmt.Errorf("spec.networkInterfaces[%d].bridge is required", i)
		}
		if ipsSeen[iface.IP] {
			return fmt.Errorf("spec.networkInterfaces[%d].ip %q is duplicated", i, iface.IP)
		}
		ipsSeen[iface.IP] = true

		// 802.1Q reserves 0 (priority-tagged frames) and 4095, leaving 1-4094 usable.
		if iface.Vlan != nil && (*iface.Vlan == 0 || *iface.Vlan > MaxVlanID) {
			return fmt.Errorf("spec.networkInterfaces[%d].vlan %d must be between 1 and %d", i, *iface.Vlan, MaxVlanID)
		}
		if err := validateVlanTrunk(i, iface); err != nil {
			return err
		}
		// A trunk has no native VLAN, so the firmware's untagged DHCP and
		// TFTP frames go nowhere and a network boot cannot work.
		if iface.PXEBoot && len(iface.VlanTrunk) > 0 {
			return fmt.Errorf("spec.networkInterfaces[%d] cannot set both pxeBoot and vlanTrunk: a trunk carries no untagged frames", i)
		}

		// Boot order and the default route are both domain-wide, so only one
		// interface may claim either.
		if iface.DefaultRoute {
			if defaultRouteIdx >= 0 {
				return fmt.Errorf("spec.networkInterfaces[%d].defaultRoute conflicts with spec.networkInterfaces[%d]: only one interface may set defaultRoute", i, defaultRouteIdx)
			}
			defaultRouteIdx = i
		}
		if iface.PXEBoot {
			if pxeBootIdx >= 0 {
				return fmt.Errorf("spec.networkInterfaces[%d].pxeBoot conflicts with spec.networkInterfaces[%d]: only one interface may set pxeBoot", i, pxeBootIdx)
			}
			pxeBootIdx = i
		}
	}

	return nil
}

// validateVlanTrunk checks a trunk interface's IDs: not beside vlan, not
// empty when given, each 1-4094 and listed once, at most MaxVlanTrunk.
func validateVlanTrunk(i int, iface v1alpha1.NetworkInterfaceSpec) error {
	if iface.VlanTrunk == nil {
		return nil
	}
	if iface.Vlan != nil {
		return fmt.Errorf("spec.networkInterfaces[%d] cannot set both vlan and vlanTrunk", i)
	}
	if len(iface.VlanTrunk) == 0 {
		return fmt.Errorf("spec.networkInterfaces[%d].vlanTrunk is empty: list the VLAN IDs, or omit it", i)
	}
	if len(iface.VlanTrunk) > MaxVlanTrunk {
		return fmt.Errorf("spec.networkInterfaces[%d].vlanTrunk lists %d IDs, at most %d", i, len(iface.VlanTrunk), MaxVlanTrunk)
	}
	seen := make(map[uint]bool, len(iface.VlanTrunk))
	for _, id := range iface.VlanTrunk {
		if id == 0 || id > MaxVlanID {
			return fmt.Errorf("spec.networkInterfaces[%d].vlanTrunk ID %d must be between 1 and %d", i, id, MaxVlanID)
		}
		if seen[id] {
			return fmt.Errorf("spec.networkInterfaces[%d].vlanTrunk lists %d twice", i, id)
		}
		seen[id] = true
	}
	return nil
}

// reservedHostDevicePrefixes are the interface name prefixes libvirt keeps
// for the names it generates: "If no target is specified, certain
// hypervisors will automatically generate a name ... the name should not
// start with either 'vnet', 'vif', 'macvtap', or 'macvlan', which are
// prefixes reserved by libvirt and certain hypervisors. Manually specified
// targets using these prefixes may be ignored"
// (https://libvirt.org/formatdomain.html#overriding-the-target-element).
// The QEMU driver drops such a name when it parses a persistent definition
// ("An auto-generated target name, blank it out", virDomainNetDefParseXML in
// https://gitlab.com/libvirt/libvirt/-/blob/v11.10.0/src/conf/domain_conf.c,
// with VIR_NET_GENERATED_VNET_PREFIX "vnet", ..._MACVTAP_PREFIX "macvtap"
// and ..._MACVLAN_PREFIX "macvlan" in src/util/virnetdev.h; "vif" is the Xen
// driver's own prefix), so the device would get a new generated name at
// every start.
var reservedHostDevicePrefixes = []string{"vnet", "vif", "macvtap", "macvlan"}

// maxHostDeviceLen is the longest Linux interface name: IFNAMSIZ (16)
// including the terminating NUL (dev_valid_name in
// https://github.com/torvalds/linux/blob/master/net/core/dev.c).
const maxHostDeviceLen = 15

// ValidateHostDevice checks a host-side interface name (spec.networkInterfaces[].hostDevice).
func ValidateHostDevice(name string) error {
	if name == "" || len(name) > maxHostDeviceLen {
		return fmt.Errorf("must be 1 to %d characters", maxHostDeviceLen)
	}
	// The kernel refuses ".", "..", "/", ":" and white space in a name
	// (dev_valid_name); foundry also keeps to printable ASCII. It also
	// refuses "%": the kernel takes a name with one as a template ("port%d")
	// and hands out the first free number (dev_alloc_name), so the device
	// would not have the name the caller looks for.
	if name == "." || name == ".." {
		return fmt.Errorf("cannot be %q", name)
	}
	for _, r := range name {
		if r <= ' ' || r >= 0x7f || r == '/' || r == ':' || r == '%' {
			return fmt.Errorf("may not contain %q (printable ASCII without '/', ':', '%%' or space)", r)
		}
	}
	for _, p := range reservedHostDevicePrefixes {
		if strings.HasPrefix(name, p) {
			return fmt.Errorf("starts with %q, a prefix libvirt reserves for the names it generates", p)
		}
	}
	return nil
}

// validateHostDevices checks spec.networkInterfaces[].hostDevice, and that
// no two interfaces end up with the same host-side name (a set one, or the
// one derived from the IP).
func validateHostDevices(vm *v1alpha1.VirtualMachine) error {
	seen := make(map[string]int)
	for i, iface := range vm.Spec.NetworkInterfaces {
		name := iface.HostDevice
		if name != "" {
			if err := ValidateHostDevice(name); err != nil {
				return fmt.Errorf("spec.networkInterfaces[%d].hostDevice %q %w", i, name, err)
			}
		} else if derived, err := naming.InterfaceNameFromIP(iface.IP); err == nil {
			name = derived
		} else {
			continue // a bad IP is reported by the interface checks
		}
		if other, ok := seen[name]; ok {
			return fmt.Errorf("spec.networkInterfaces[%d] has the host device name %q of spec.networkInterfaces[%d]", i, name, other)
		}
		seen[name] = i
	}
	return nil
}

// maxDiskSerialLen is the longest disk serial accepted: a virtio-blk serial
// holds at most 20 bytes, and QEMU silently truncates a longer one, which
// would let two distinct serials reach the guest as the same name.
const maxDiskSerialLen = 20

// diskSerialRE is the character set accepted in a disk serial: letters,
// digits, '-', '_' and '.', which pass unchanged through libvirt, QEMU and
// the guest's /dev/disk/by-id names.
var diskSerialRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateDiskSerials checks spec.bootDisk.serial and spec.dataDisks[].serial:
// each set serial is 1-20 characters from A-Z a-z 0-9 - _ . and no two disks
// of the VM share one.
func validateDiskSerials(vm *v1alpha1.VirtualMachine) error {
	seen := make(map[string]string)
	check := func(field, serial string) error {
		if serial == "" {
			return nil
		}
		if len(serial) > maxDiskSerialLen {
			return fmt.Errorf("%s %q is longer than %d characters", field, serial, maxDiskSerialLen)
		}
		if !diskSerialRE.MatchString(serial) {
			return fmt.Errorf("%s %q may only contain the characters [A-Za-z0-9._-]", field, serial)
		}
		if other, ok := seen[serial]; ok {
			return fmt.Errorf("%s %q is already used by %s", field, serial, other)
		}
		seen[serial] = field
		return nil
	}
	if err := check("spec.bootDisk.serial", vm.Spec.BootDisk.Serial); err != nil {
		return err
	}
	for i, disk := range vm.Spec.DataDisks {
		if err := check(fmt.Sprintf("spec.dataDisks[%d].serial", i), disk.Serial); err != nil {
			return err
		}
	}
	return nil
}

// validateFirmware checks spec.firmware.
func validateFirmware(vm *v1alpha1.VirtualMachine) error {
	fw := vm.Spec.Firmware
	if fw == nil {
		return nil
	}
	if fw.Loader == "" {
		return fmt.Errorf("spec.firmware.loader is required")
	}
	// Secure Boot needs SMM to protect the varstore, which only x86 has.
	if fw.SecureBoot && vm.GetArch() != v1alpha1.ArchX86_64 {
		return fmt.Errorf("spec.firmware.secureBoot is only supported for %s", v1alpha1.ArchX86_64)
	}
	if fw.Keys != "" {
		if fw.NVRAMTemplate != "" {
			return fmt.Errorf("spec.firmware.keys and spec.firmware.nvramTemplate are mutually exclusive")
		}
		if !ValidKeyProfileName(fw.Keys) {
			return fmt.Errorf("spec.firmware.keys %q must be at most 63 lowercase letters, digits and dashes, starting and ending with a letter or digit", fw.Keys)
		}
	}
	return nil
}

// keyProfileName is the form of a Secure Boot key profile name. It becomes
// part of a volume name (naming.VolumeNameVarstore), so it has no "_" or "/".
var keyProfileName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidKeyProfileName reports whether name can name a Secure Boot key profile.
func ValidKeyProfileName(name string) bool {
	return len(name) <= 63 && keyProfileName.MatchString(name)
}

// serialRE is the form of spec.serial and of the default serial.
var serialRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// smbiosStringRE is the form of a spec.smbios string: the characters vendors
// use in these fields, without leading or trailing space (which tools that
// read SMBIOS strip, so the guest would see a different value) and without
// quotes, commas or '=', which would need escaping on QEMU's -smbios option.
var smbiosStringRE = regexp.MustCompile(`^[A-Za-z0-9._+()/:-]([A-Za-z0-9 ._+()/:-]{0,62}[A-Za-z0-9._+()/:-])?$`)

// uuidRE is the 8-4-4-4-12 hexadecimal UUID form (after lower-casing).
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validateSMBIOS checks spec.serial and spec.smbios. The serial goes into
// SMBIOS through libvirt's sysinfo; a narrow character set keeps it a plain
// token on every side (libvirt XML, QEMU's -smbios option, the guest's sysfs).
func validateSMBIOS(vm *v1alpha1.VirtualMachine) error {
	if !vm.HasSMBIOS() {
		return nil
	}
	if s := vm.GetSerial(); !serialRE.MatchString(s) {
		return fmt.Errorf("spec.serial %q must be 1 to 64 characters from A-Z a-z 0-9 . _ - (the default is %q + metadata.name)", s, v1alpha1.SerialPrefix)
	}
	s := vm.Spec.SMBIOS
	if s == nil {
		return nil
	}
	for _, f := range []struct{ field, value string }{
		{"manufacturer", s.Manufacturer},
		{"product", s.Product},
		{"version", s.Version},
		{"sku", s.SKU},
		{"family", s.Family},
	} {
		if f.value != "" && !smbiosStringRE.MatchString(f.value) {
			return fmt.Errorf("spec.smbios.%s %q must be 1 to 64 characters from A-Z a-z 0-9, space and . _ - + ( ) / :, not starting or ending with a space", f.field, f.value)
		}
	}
	if s.UUID != "" && !uuidRE.MatchString(s.UUID) {
		return fmt.Errorf("spec.smbios.uuid %q is not a UUID in the form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", s.UUID)
	}
	return nil
}

// fwCfgMaxName is the longest fw_cfg name QEMU takes: FW_CFG_MAX_FILE_PATH is
// 56 bytes including the terminating NUL
// (https://gitlab.com/qemu-project/qemu/-/blob/v10.1.0/include/standard-headers/linux/qemu_fw_cfg.h).
// A longer name passes libvirt's define and fails when QEMU starts.
const fwCfgMaxName = 55

// fwCfgMaxString is the longest fw_cfg string entry. libvirt hands it to QEMU
// as one command-line argument, "name=<name>,string=<value>"
// (https://libvirt.org/formatdomain.html#smbios-system-information), and
// exec refuses an argument of MAX_ARG_STRLEN (32 pages, 128 KiB with 4 KiB
// pages, its NUL included) or longer with E2BIG
// (https://man7.org/linux/man-pages/man2/execve.2.html, "Limits on size of
// arguments and environment"), so a longer string passes the define and
// fails when the VM starts. 128 bytes are left for the rest of the argument;
// larger content belongs in a file entry.
const fwCfgMaxString = 128<<10 - 128

// fwCfgStringComma explains why a comma in a fw_cfg string is refused.
// libvirt (seen with 11.3) builds the QEMU argument
// "name=<name>,string=<value>" without escaping commas in the value, and QEMU
// splits its options on commas, so a value with a comma passes the define and
// fails when the VM starts. libvirt passes a file entry by path instead.
const fwCfgStringComma = "contains a comma, which libvirt passes to QEMU unescaped, so the VM would fail to start; put the content in a file entry instead"

// validateFwCfg checks a list of fw_cfg entries.
//
// libvirt only accepts names under "opt/" and reserves "opt/ovmf/" and
// "opt/org.qemu/" for itself (<sysinfo type='fwcfg'>,
// https://libvirt.org/formatdomain.html#smbios-system-information); checking
// here fails at load time rather than at define time.
func validateFwCfg(field string, entries []v1alpha1.FwCfgEntrySpec) error {
	seen := make(map[string]bool)
	for i, e := range entries {
		if !strings.HasPrefix(e.Name, "opt/") {
			return fmt.Errorf("%s[%d].name %q must start with \"opt/\"", field, i, e.Name)
		}
		if len(e.Name) > fwCfgMaxName {
			return fmt.Errorf("%s[%d].name %q is %d characters; QEMU allows at most %d", field, i, e.Name, len(e.Name), fwCfgMaxName)
		}
		if strings.HasPrefix(e.Name, "opt/ovmf/") || strings.HasPrefix(e.Name, "opt/org.qemu/") {
			return fmt.Errorf("%s[%d].name %q is reserved by libvirt", field, i, e.Name)
		}
		if (e.File == "") == (e.String == "") {
			return fmt.Errorf("%s[%d] requires exactly one of file or string", field, i)
		}
		if len(e.String) > fwCfgMaxString {
			return fmt.Errorf("%s[%d].string is %d bytes; at most %d fit in the QEMU argument libvirt passes it in, put larger content in a file", field, i, len(e.String), fwCfgMaxString)
		}
		if strings.Contains(e.String, ",") {
			return fmt.Errorf("%s[%d].string %s", field, i, fwCfgStringComma)
		}
		if seen[e.Name] {
			return fmt.Errorf("%s[%d].name %q is duplicated", field, i, e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}

// maxInputStringBytes bounds the inline content (string) of all input files
// together. The spec, inline content included, is stored in the domain's
// metadata, and libvirt takes at most 4 MiB in one string over its RPC
// protocol (REMOTE_STRING_MAX); larger content belongs in a file.
const maxInputStringBytes = 1 << 20

// inputLabelRE is the form of an input disk label: an ISO-9660 volume
// identifier of d-characters (ECMA-119 7.4.1), at most 32 of them.
var inputLabelRE = regexp.MustCompile(`^[A-Z0-9_]{1,32}$`)

// inputFileNameRE is the form of an input file name that reaches a Linux
// guest unchanged on a plain ISO-9660 image: lower case (the image writer
// lower-cases, isofs shows lower case), a-z 0-9 _ - (other characters become
// "_"), at most one dot (the writer turns any other dot into "_"), and an
// extension of at most 8 characters (the writer cuts a longer one).
var inputFileNameRE = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]{1,8})?$`)

// maxInputFileNameLen is the longest input file name: ISO-9660 file
// identifiers hold 30 characters, including the ";1" version suffix.
const maxInputFileNameLen = 28

// validateInputDisks checks spec.inputDisks. File contents are read, and
// their size checked, when the disk images are built (vm.Create).
func validateInputDisks(vm *v1alpha1.VirtualMachine) error {
	if len(vm.Spec.InputDisks) > naming.MaxInputDisks {
		return fmt.Errorf("spec.inputDisks has %d disks; at most %d are supported (their CD-ROMs take sdb to sdz)", len(vm.Spec.InputDisks), naming.MaxInputDisks)
	}
	labels := make(map[string]int)
	inline := 0
	for i, d := range vm.Spec.InputDisks {
		field := fmt.Sprintf("spec.inputDisks[%d]", i)
		if !inputLabelRE.MatchString(d.Label) {
			return fmt.Errorf("%s.label %q must be 1 to 32 characters from A-Z 0-9 _", field, d.Label)
		}
		if other, ok := labels[d.Label]; ok {
			return fmt.Errorf("%s.label %q is already used by spec.inputDisks[%d]", field, d.Label, other)
		}
		labels[d.Label] = i
		if d.Label == "CIDATA" && vm.Spec.CloudInit != nil {
			return fmt.Errorf("%s.label CIDATA is the label of the cloud-init seed; it is only allowed without spec.cloudInit", field)
		}
		if len(d.Files) == 0 {
			return fmt.Errorf("%s.files must list at least one file", field)
		}
		names := make(map[string]bool)
		for j, f := range d.Files {
			ff := fmt.Sprintf("%s.files[%d]", field, j)
			if len(f.Name) > maxInputFileNameLen || !inputFileNameRE.MatchString(f.Name) {
				return fmt.Errorf("%s.name %q must be at most %d characters: a-z 0-9 _ - and an optional extension of a dot and 1 to 8 of the same", ff, f.Name, maxInputFileNameLen)
			}
			if names[f.Name] {
				return fmt.Errorf("%s.name %q is duplicated", ff, f.Name)
			}
			names[f.Name] = true
			if (f.File == "") == (f.String == "") {
				return fmt.Errorf("%s requires exactly one of file or string", ff)
			}
			inline += len(f.String)
		}
	}
	// libvirt refuses two disks with the same target at define time
	for i := range vm.Spec.InputDisks {
		target := naming.InputDiskTarget(i)
		for j, disk := range vm.Spec.DataDisks {
			if disk.Device == target {
				return fmt.Errorf("spec.dataDisks[%d].device %q is the target of spec.inputDisks[%d]", j, disk.Device, i)
			}
		}
	}
	if inline > maxInputStringBytes {
		return fmt.Errorf("spec.inputDisks inline strings total %d bytes; at most %d (1 MiB) are allowed, put larger content in a file", inline, maxInputStringBytes)
	}
	return nil
}

// validateImageSHA256 checks spec.bootDisk.imageSHA256: 64 lower-case hex
// digits, as sha256sum prints them, and only for an image volume in
// foundry-images. Create checks the digest by reading that volume back
// through libvirt; a file path is not a volume, and create takes the image
// from foundry-images whatever pool the spec names, so a digest for another
// pool would be checked against a volume other than the one booted.
func validateImageSHA256(disk v1alpha1.BootDiskSpec) error {
	d := disk.ImageSHA256
	if d == "" {
		return nil
	}
	img := disk.Image
	if img == "" || strings.HasPrefix(img, ".") || strings.Contains(img, "/") {
		return fmt.Errorf("spec.bootDisk.imageSHA256 goes with an image volume, not an empty disk or a file path")
	}
	if !isSHA256(d) {
		return fmt.Errorf("spec.bootDisk.imageSHA256 %q: must be 64 lower-case hex digits", d)
	}
	pool := disk.ImagePool
	if p, _, ok := strings.Cut(img, ":"); ok {
		pool = p
	}
	if pool = strings.TrimSpace(pool); pool != "" && pool != imagesPool {
		return fmt.Errorf("spec.bootDisk.imageSHA256: the image is in pool %q; digests are checked for images in %s", pool, imagesPool)
	}
	return nil
}

// isSHA256 reports whether s is a sha256 digest in lower-case hex.
func isSHA256(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}
