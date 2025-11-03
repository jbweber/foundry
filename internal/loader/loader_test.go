package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestLoadFromYAML_Valid(t *testing.T) {
	yaml := `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	vm, err := LoadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}

	// Verify basic fields
	if vm.Name != "test-vm" {
		t.Errorf("Expected name 'test-vm', got %s", vm.Name)
	}
	if vm.Spec.VCPUs != 2 {
		t.Errorf("Expected VCPUs 2, got %d", vm.Spec.VCPUs)
	}
	if vm.Spec.MemoryGiB != 4 {
		t.Errorf("Expected MemoryGiB 4, got %d", vm.Spec.MemoryGiB)
	}

	// Verify defaults were applied
	if vm.Spec.CPUMode != "host-model" {
		t.Errorf("Expected default CPUMode 'host-model', got %s", vm.Spec.CPUMode)
	}
	if vm.Spec.StoragePool != "foundry-vms" {
		t.Errorf("Expected default StoragePool 'foundry-vms', got %s", vm.Spec.StoragePool)
	}
	if vm.Spec.BootDisk.Format != "qcow2" {
		t.Errorf("Expected default BootDisk.Format 'qcow2', got %s", vm.Spec.BootDisk.Format)
	}
	if vm.Spec.BootDisk.ImagePool != "foundry-images" {
		t.Errorf("Expected default BootDisk.ImagePool 'foundry-images', got %s", vm.Spec.BootDisk.ImagePool)
	}
	if vm.Spec.Autostart == nil || !*vm.Spec.Autostart {
		t.Error("Expected default Autostart to be true")
	}
	if vm.Status.Phase != v1alpha1.VMPhasePending {
		t.Errorf("Expected default Phase 'Pending', got %s", vm.Status.Phase)
	}
}

func TestLoadFromYAML_MissingAPIVersion(t *testing.T) {
	yaml := `
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("Expected error for missing apiVersion")
	}
}

func TestLoadFromYAML_MissingKind(t *testing.T) {
	yaml := `
apiVersion: foundry.cofront.xyz/v1alpha1
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("Expected error for missing kind")
	}
}

func TestLoadFromYAML_WrongAPIVersion(t *testing.T) {
	yaml := `
apiVersion: wrong.api/v1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("Expected error for wrong apiVersion")
	}
}

func TestLoadFromYAML_WrongKind(t *testing.T) {
	yaml := `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: WrongKind
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("Expected error for wrong kind")
	}
}

func TestLoadFromYAML_InvalidYAML(t *testing.T) {
	yaml := `{invalid yaml content`

	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "vm.yaml")

	content := `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	if err := os.WriteFile(yamlPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	vm, err := LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if vm.Name != "test-vm" {
		t.Errorf("Expected name 'test-vm', got %s", vm.Name)
	}
}

func TestLoadFromFile_NonExistent(t *testing.T) {
	_, err := LoadFromFile("/non/existent/file.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestSaveToFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "vm.yaml")

	vm := v1alpha1.NewVirtualMachine("test-vm")
	vm.Spec.VCPUs = 2
	vm.Spec.MemoryGiB = 4
	vm.Spec.BootDisk.SizeGB = 50
	vm.Spec.BootDisk.Image = "fedora-43.qcow2"
	vm.Spec.NetworkInterfaces = []v1alpha1.NetworkInterfaceSpec{
		{
			IP:      "10.0.0.1/24",
			Gateway: "10.0.0.254",
			Bridge:  "br0",
		},
	}

	err := SaveToFile(vm, yamlPath)
	if err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// Load it back and verify
	loaded, err := LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("Failed to load saved file: %v", err)
	}

	if loaded.Name != vm.Name {
		t.Errorf("Name mismatch after round-trip")
	}
	if loaded.Spec.VCPUs != vm.Spec.VCPUs {
		t.Errorf("VCPUs mismatch after round-trip")
	}
}

func TestSaveToFile_MissingAPIVersion(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "vm.yaml")

	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}
	// Don't set APIVersion/Kind - should be added automatically by SaveToFile

	err := SaveToFile(vm, yamlPath)
	if err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Load it back and verify TypeMeta was set
	loaded, err := LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("Failed to load saved file: %v", err)
	}

	if loaded.APIVersion != "foundry.cofront.xyz/v1alpha1" {
		t.Errorf("Expected apiVersion to be set automatically")
	}
	if loaded.Kind != "VirtualMachine" {
		t.Errorf("Expected kind to be set automatically")
	}
}

func TestApplyDefaults(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "TEST-VM",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			CloudInit: &v1alpha1.CloudInitSpec{
				FQDN: "TEST.EXAMPLE.COM",
			},
		},
	}

	applyDefaults(vm)

	// Check defaults
	if vm.Spec.CPUMode != "host-model" {
		t.Errorf("Expected default CPUMode, got %s", vm.Spec.CPUMode)
	}
	if vm.Spec.StoragePool != "foundry-vms" {
		t.Errorf("Expected default StoragePool, got %s", vm.Spec.StoragePool)
	}
	if vm.Spec.BootDisk.Format != "qcow2" {
		t.Errorf("Expected default BootDisk.Format, got %s", vm.Spec.BootDisk.Format)
	}
	if vm.Spec.BootDisk.ImagePool != "foundry-images" {
		t.Errorf("Expected default BootDisk.ImagePool, got %s", vm.Spec.BootDisk.ImagePool)
	}
	if vm.Spec.Autostart == nil {
		t.Error("Expected Autostart to be set")
	}
	if vm.Status.Phase != v1alpha1.VMPhasePending {
		t.Errorf("Expected default Phase, got %s", vm.Status.Phase)
	}

	// Check normalization
	if vm.Name != "test-vm" {
		t.Errorf("Expected name to be lowercased, got %s", vm.Name)
	}
	if vm.Spec.CloudInit.FQDN != "test.example.com" {
		t.Errorf("Expected FQDN to be lowercased, got %s", vm.Spec.CloudInit.FQDN)
	}
}

func TestValidateSpec_Valid(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "test-vm",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:      "10.0.0.1/24",
					Gateway: "10.0.0.254",
					Bridge:  "br0",
				},
			},
		},
	}

	if err := validateSpec(vm); err != nil {
		t.Errorf("Expected valid spec, got error: %v", err)
	}
}

func TestValidateSpec_MissingName(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for missing name")
	}
}

func TestValidateSpec_InvalidVCPUs(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     0,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for invalid VCPUs")
	}
}

func TestValidateSpec_InvalidMemory(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 0,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for invalid memory")
	}
}

func TestValidateSpec_InvalidBootDiskSize(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 0,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for invalid boot disk size")
	}
}

func TestValidateSpec_BootDiskNoImageNoEmpty(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				// No image and empty=false
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error when boot disk has no image and empty=false")
	}
}

func TestValidateSpec_BootDiskBothImageAndEmpty(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
				Empty:  true, // Can't have both
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error when boot disk has both image and empty=true")
	}
}

func TestValidateSpec_DuplicateDataDiskDevice(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			DataDisks: []v1alpha1.DataDiskSpec{
				{Device: "vdb", SizeGB: 100},
				{Device: "vdb", SizeGB: 200}, // duplicate
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for duplicate data disk device")
	}
}

func TestValidateSpec_DataDiskMissingDevice(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			DataDisks: []v1alpha1.DataDiskSpec{
				{SizeGB: 100}, // missing device
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for data disk missing device")
	}
}

func TestValidateSpec_DataDiskInvalidSize(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			DataDisks: []v1alpha1.DataDiskSpec{
				{Device: "vdb", SizeGB: 0}, // invalid size
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for data disk invalid size")
	}
}

func TestValidateSpec_NoNetworkInterfaces(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for no network interfaces")
	}
}

func TestValidateSpec_NetworkInterfaceMissingFields(t *testing.T) {
	tests := []struct {
		name  string
		iface v1alpha1.NetworkInterfaceSpec
	}{
		{"missing IP", v1alpha1.NetworkInterfaceSpec{Gateway: "10.0.0.254", Bridge: "br0"}},
		{"missing Gateway", v1alpha1.NetworkInterfaceSpec{IP: "10.0.0.1/24", Bridge: "br0"}},
		{"missing Bridge", v1alpha1.NetworkInterfaceSpec{IP: "10.0.0.1/24", Gateway: "10.0.0.254"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "fedora-43.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{tt.iface},
				},
			}

			if err := validateSpec(vm); err == nil {
				t.Errorf("Expected error for %s", tt.name)
			}
		})
	}
}

func TestValidateSpec_DuplicateIP(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 50,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br1"}, // duplicate IP
			},
		},
	}

	if err := validateSpec(vm); err == nil {
		t.Error("Expected error for duplicate IP")
	}
}
