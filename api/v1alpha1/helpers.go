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

// GetCPUMode returns the CPU mode with default fallback.
func (vm *VirtualMachine) GetCPUMode() string {
	if vm.Spec.CPUMode == "" {
		return "host-model"
	}
	return vm.Spec.CPUMode
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
