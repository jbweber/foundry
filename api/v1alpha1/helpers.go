package v1alpha1

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// GroupName is the API group for Foundry resources.
	GroupName = "foundry.cofront.xyz"

	// Version is the API version.
	Version = "v1alpha1"

	// VirtualMachineKind is the kind string for VirtualMachine resources.
	VirtualMachineKind = "VirtualMachine"
)

// Guest architectures, using libvirt's names.
const (
	// ArchX86_64 is the 64-bit x86 architecture, and the default.
	ArchX86_64 = "x86_64"

	// ArchAarch64 is the 64-bit Arm architecture.
	ArchAarch64 = "aarch64"
)

// Libvirt domain types.
const (
	// DomainTypeKVM runs the guest with KVM hardware virtualization.
	DomainTypeKVM = "kvm"

	// DomainTypeQEMU runs the guest under QEMU TCG emulation.
	DomainTypeQEMU = "qemu"
)

// CPU modes.
const (
	// CPUModeHostModel gives the guest a named CPU model close to the host's.
	CPUModeHostModel = "host-model"

	// CPUModeHostPassthrough gives the guest the host's CPU as it is.
	CPUModeHostPassthrough = "host-passthrough"

	// CPUModeMaximum gives the guest every feature the hypervisor can offer;
	// under emulation, everything QEMU implements.
	CPUModeMaximum = "maximum"
)

// ArchFromGOARCH maps a Go architecture name to libvirt's name for it.
// Unknown values are returned unchanged.
func ArchFromGOARCH(goarch string) string {
	switch goarch {
	case "amd64":
		return ArchX86_64
	case "arm64":
		return ArchAarch64
	default:
		return goarch
	}
}

// NewVirtualMachine creates a new VirtualMachine with TypeMeta and ObjectMeta defaults.
func NewVirtualMachine(name string) *VirtualMachine {
	now := Time{Time: time.Now()}
	autostart := true

	return &VirtualMachine{
		TypeMeta: TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       VirtualMachineKind,
		},
		ObjectMeta: ObjectMeta{
			Name:              name,
			UID:               uuid.New().String(),
			CreationTimestamp: now,
			Generation:        1,
		},
		Spec: VirtualMachineSpec{
			CPUMode:     "host-model",
			StoragePool: "foundry-vms",
			Autostart:   &autostart,
			BootDisk: BootDiskSpec{
				ImagePool: "foundry-images",
				Format:    "qcow2",
			},
		},
		Status: VirtualMachineStatus{
			Phase: VMPhasePending,
		},
	}
}

// SetDefaultAPIVersion ensures the VM has the correct apiVersion and kind.
// Useful when loading from files that might be missing these fields.
func SetDefaultAPIVersion(vm *VirtualMachine) {
	if vm.APIVersion == "" {
		vm.APIVersion = GroupName + "/" + Version
	}
	if vm.Kind == "" {
		vm.Kind = VirtualMachineKind
	}
}

// IsAutostart returns true if the VM is configured to autostart.
// Handles nil pointer by returning default value (true).
func (vm *VirtualMachine) IsAutostart() bool {
	if vm.Spec.Autostart == nil {
		return true // default
	}
	return *vm.Spec.Autostart
}

// GetArch returns the guest architecture, x86_64 when spec.arch is unset.
func (vm *VirtualMachine) GetArch() string {
	if vm.Spec.Arch == "" {
		return ArchX86_64
	}
	return vm.Spec.Arch
}

// SerialPrefix starts the default SMBIOS system serial: SerialPrefix + metadata.name.
const SerialPrefix = "foundry-"

// HasSMBIOS reports whether the VM gets SMBIOS system information: only when
// spec.serial or spec.smbios is set, so a spec with neither renders the same
// domain as before these fields existed.
func (vm *VirtualMachine) HasSMBIOS() bool {
	return vm.Spec.Serial != "" || vm.Spec.SMBIOS != nil
}

// GetSerial returns the VM's SMBIOS system serial: spec.serial, or
// SerialPrefix + metadata.name. It is used only when HasSMBIOS is true.
func (vm *VirtualMachine) GetSerial() string {
	if vm.Spec.Serial != "" {
		return vm.Spec.Serial
	}
	return SerialPrefix + vm.Name
}

// GetDomainType returns the libvirt domain type for running the VM on a
// hypervisor of architecture hostArch (libvirt's name): KVM when the guest
// has the host's architecture, QEMU emulation otherwise. A spec without
// spec.arch always gets KVM, as before spec.arch existed.
func (vm *VirtualMachine) GetDomainType(hostArch string) string {
	if vm.Spec.Arch == "" || vm.Spec.Arch == hostArch {
		return DomainTypeKVM
	}
	return DomainTypeQEMU
}

// GetCPUMode returns the CPU mode with default fallback, for the guest
// running under KVM on a host of its own architecture. CPUModeFor gives the
// mode for a given domain type.
func (vm *VirtualMachine) GetCPUMode() string {
	return vm.CPUModeFor(DomainTypeKVM)
}

// CPUModeFor returns the CPU mode with default fallback for a domain type.
// See defaultCPUMode.
func (vm *VirtualMachine) CPUModeFor(domainType string) string {
	if vm.Spec.CPUMode == "" {
		return defaultCPUMode(vm.GetArch(), domainType)
	}
	return vm.Spec.CPUMode
}

// defaultCPUMode picks the CPU mode for an architecture and domain type.
//
// An emulated guest gets maximum: host-model and host-passthrough describe
// the host's CPU, which TCG does not offer. Under KVM, x86_64 keeps
// host-model. QEMU implements host-model only for x86 (and ppc64), so an
// aarch64 guest gets host-passthrough, the mode RHEL supports for Arm guests:
// https://libvirt.org/formatdomain.html#cpu-model-and-topology
// https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/10/html/configuring_and_managing_linux_virtual_machines/feature-support-and-limitations-in-rhel-10-virtualization
func defaultCPUMode(arch, domainType string) string {
	switch {
	case domainType == DomainTypeQEMU:
		return CPUModeMaximum
	case arch == ArchAarch64:
		return CPUModeHostPassthrough
	default:
		return CPUModeHostModel
	}
}

// GetStoragePool returns the storage pool with default fallback.
func (vm *VirtualMachine) GetStoragePool() string {
	if vm.Spec.StoragePool == "" {
		return "foundry-vms"
	}
	return vm.Spec.StoragePool
}

// GetBootDiskFormat returns the boot disk format with default fallback.
func (vm *VirtualMachine) GetBootDiskFormat() string {
	if vm.Spec.BootDisk.Format == "" {
		return "qcow2"
	}
	return vm.Spec.BootDisk.Format
}

// GetBootDiskImagePool returns the boot disk image pool with default fallback.
func (vm *VirtualMachine) GetBootDiskImagePool() string {
	if vm.Spec.BootDisk.ImagePool == "" {
		return "foundry-images"
	}
	return vm.Spec.BootDisk.ImagePool
}

// GetName returns the VM name from metadata.
func (vm *VirtualMachine) GetName() string {
	return vm.Name
}

// SetPhase sets the VM phase in status.
func (vm *VirtualMachine) SetPhase(phase VMPhase) {
	vm.Status.Phase = phase
}

// GetPhase returns the current VM phase.
func (vm *VirtualMachine) GetPhase() VMPhase {
	return vm.Status.Phase
}

// SetDomainUUID sets the libvirt domain UUID in status.
func (vm *VirtualMachine) SetDomainUUID(uuid string) {
	vm.Status.DomainUUID = uuid
}

// GetDomainUUID returns the libvirt domain UUID.
func (vm *VirtualMachine) GetDomainUUID() string {
	return vm.Status.DomainUUID
}

// AddAddress adds a network address to the status.
func (vm *VirtualMachine) AddAddress(addrType, address string) {
	vm.Status.Addresses = append(vm.Status.Addresses, VMAddress{
		Type:    addrType,
		Address: address,
	})
}

// SetMACAddresses sets the MAC addresses in status.
func (vm *VirtualMachine) SetMACAddresses(macs []string) {
	vm.Status.MACAddresses = macs
}

// GetMACAddresses returns the MAC addresses from status.
func (vm *VirtualMachine) GetMACAddresses() []string {
	return vm.Status.MACAddresses
}

// SetInterfaceNames sets the interface names in status.
func (vm *VirtualMachine) SetInterfaceNames(names []string) {
	vm.Status.InterfaceNames = names
}

// GetInterfaceNames returns the interface names from status.
func (vm *VirtualMachine) GetInterfaceNames() []string {
	return vm.Status.InterfaceNames
}

// UpdateObservedGeneration updates the status.observedGeneration to match metadata.generation.
func (vm *VirtualMachine) UpdateObservedGeneration() {
	vm.Status.ObservedGeneration = vm.Generation
}

// GetBootVolumeName returns the volume name for the boot disk.
// Format: <vm-name>_boot.qcow2 (includes extension to indicate format)
func (vm *VirtualMachine) GetBootVolumeName() string {
	return fmt.Sprintf("%s_boot.qcow2", vm.Name)
}

// GetDataVolumeName returns the volume name for a data disk.
// Format: <vm-name>_data-<device>.qcow2 (e.g., "my-vm_data-vdb.qcow2")
func (vm *VirtualMachine) GetDataVolumeName(device string) string {
	return fmt.Sprintf("%s_data-%s.qcow2", vm.Name, device)
}

// GetCloudInitVolumeName returns the volume name for the cloud-init ISO.
// Format: <vm-name>_cloudinit.iso (includes extension to indicate format)
func (vm *VirtualMachine) GetCloudInitVolumeName() string {
	return fmt.Sprintf("%s_cloudinit.iso", vm.Name)
}

// Normalize sanitizes user input to consistent formats.
// This is called automatically before validation.
func (vm *VirtualMachine) Normalize() {
	// Normalize VM name to lowercase
	vm.Name = strings.ToLower(strings.TrimSpace(vm.Name))

	// Normalize cloud-init FQDN to lowercase (hostname will be derived from this)
	if vm.Spec.CloudInit != nil {
		vm.Spec.CloudInit.FQDN = strings.ToLower(strings.TrimSpace(vm.Spec.CloudInit.FQDN))
	}

	// Note: Bridge names are NOT normalized - they must match hypervisor config exactly

	// Set default storage pool if not specified
	if vm.Spec.StoragePool == "" {
		vm.Spec.StoragePool = "foundry-vms"
	}

	// Set default image pool if not specified
	if vm.Spec.BootDisk.ImagePool == "" {
		vm.Spec.BootDisk.ImagePool = "foundry-images"
	}
}
