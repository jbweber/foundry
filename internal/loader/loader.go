// Package loader provides functions for loading VirtualMachine resources
// from YAML files.
package loader

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// LoadFromFile loads a VirtualMachine resource from a YAML file.
// The file must be in the foundry.cofront.xyz/v1alpha1 format.
func LoadFromFile(path string) (*v1alpha1.VirtualMachine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return LoadFromYAML(data)
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

	data, err := yaml.Marshal(vm)
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
	// Apply defaults from helpers
	if vm.Spec.CPUMode == "" {
		vm.Spec.CPUMode = "host-model"
	}
	if vm.Spec.StoragePool == "" {
		vm.Spec.StoragePool = "foundry-vms"
	}
	if vm.Spec.BootDisk.Format == "" {
		vm.Spec.BootDisk.Format = "qcow2"
	}
	if vm.Spec.BootDisk.ImagePool == "" {
		vm.Spec.BootDisk.ImagePool = "foundry-images"
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
}

// validateSpec validates the VirtualMachine spec for required fields and consistency.
func validateSpec(vm *v1alpha1.VirtualMachine) error {
	// Validate metadata.name
	if vm.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	// Validate VCPUs
	if vm.Spec.VCPUs <= 0 {
		return fmt.Errorf("spec.vcpus must be greater than 0")
	}

	// Validate memory
	if vm.Spec.MemoryGiB <= 0 {
		return fmt.Errorf("spec.memoryGiB must be greater than 0")
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

	// Validate data disks
	devicesSeen := make(map[string]bool)
	for i, disk := range vm.Spec.DataDisks {
		if disk.Device == "" {
			return fmt.Errorf("spec.dataDisks[%d].device is required", i)
		}
		if disk.SizeGB <= 0 {
			return fmt.Errorf("spec.dataDisks[%d].sizeGB must be greater than 0", i)
		}
		if devicesSeen[disk.Device] {
			return fmt.Errorf("spec.dataDisks[%d].device %q is duplicated", i, disk.Device)
		}
		devicesSeen[disk.Device] = true
	}

	// Validate network interfaces
	if len(vm.Spec.NetworkInterfaces) == 0 {
		return fmt.Errorf("spec.networkInterfaces must have at least one interface")
	}

	ipsSeen := make(map[string]bool)
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
	}

	return nil
}
