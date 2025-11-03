package v1alpha1

import (
	"testing"
)

func TestNewVirtualMachine(t *testing.T) {
	name := "test-vm"
	vm := NewVirtualMachine(name)

	// Verify TypeMeta
	if vm.APIVersion != "foundry.cofront.xyz/v1alpha1" {
		t.Errorf("Expected APIVersion 'foundry.cofront.xyz/v1alpha1', got %s", vm.APIVersion)
	}
	if vm.Kind != "VirtualMachine" {
		t.Errorf("Expected Kind 'VirtualMachine', got %s", vm.Kind)
	}

	// Verify ObjectMeta
	if vm.Name != name {
		t.Errorf("Expected Name %s, got %s", name, vm.Name)
	}
	if vm.UID == "" {
		t.Error("Expected UID to be set, got empty string")
	}
	if vm.Generation != 1 {
		t.Errorf("Expected Generation 1, got %d", vm.Generation)
	}
	if vm.CreationTimestamp.IsZero() {
		t.Error("Expected CreationTimestamp to be set")
	}

	// Verify Spec defaults
	if vm.Spec.CPUMode != "host-model" {
		t.Errorf("Expected CPUMode 'host-model', got %s", vm.Spec.CPUMode)
	}
	if vm.Spec.StoragePool != "foundry-vms" {
		t.Errorf("Expected StoragePool 'foundry-vms', got %s", vm.Spec.StoragePool)
	}
	if vm.Spec.Autostart == nil {
		t.Error("Expected Autostart to be set")
	} else if !*vm.Spec.Autostart {
		t.Error("Expected Autostart to be true by default")
	}
	if vm.Spec.BootDisk.ImagePool != "foundry-images" {
		t.Errorf("Expected BootDisk.ImagePool 'foundry-images', got %s", vm.Spec.BootDisk.ImagePool)
	}
	if vm.Spec.BootDisk.Format != "qcow2" {
		t.Errorf("Expected BootDisk.Format 'qcow2', got %s", vm.Spec.BootDisk.Format)
	}

	// Verify Status defaults
	if vm.Status.Phase != VMPhasePending {
		t.Errorf("Expected Phase 'Pending', got %s", vm.Status.Phase)
	}
}

func TestSetDefaultAPIVersion(t *testing.T) {
	tests := []struct {
		name         string
		vm           *VirtualMachine
		expectedAPI  string
		expectedKind string
	}{
		{
			name: "missing both",
			vm: &VirtualMachine{
				TypeMeta: TypeMeta{},
			},
			expectedAPI:  "foundry.cofront.xyz/v1alpha1",
			expectedKind: "VirtualMachine",
		},
		{
			name: "missing apiVersion only",
			vm: &VirtualMachine{
				TypeMeta: TypeMeta{
					Kind: "VirtualMachine",
				},
			},
			expectedAPI:  "foundry.cofront.xyz/v1alpha1",
			expectedKind: "VirtualMachine",
		},
		{
			name: "missing kind only",
			vm: &VirtualMachine{
				TypeMeta: TypeMeta{
					APIVersion: "foundry.cofront.xyz/v1alpha1",
				},
			},
			expectedAPI:  "foundry.cofront.xyz/v1alpha1",
			expectedKind: "VirtualMachine",
		},
		{
			name: "both already set",
			vm: &VirtualMachine{
				TypeMeta: TypeMeta{
					APIVersion: "custom/v1",
					Kind:       "CustomKind",
				},
			},
			expectedAPI:  "custom/v1",
			expectedKind: "CustomKind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultAPIVersion(tt.vm)
			if tt.vm.APIVersion != tt.expectedAPI {
				t.Errorf("Expected APIVersion %s, got %s", tt.expectedAPI, tt.vm.APIVersion)
			}
			if tt.vm.Kind != tt.expectedKind {
				t.Errorf("Expected Kind %s, got %s", tt.expectedKind, tt.vm.Kind)
			}
		})
	}
}

func TestIsAutostart(t *testing.T) {
	tests := []struct {
		name      string
		autostart *bool
		expected  bool
	}{
		{
			name:      "nil pointer defaults to true",
			autostart: nil,
			expected:  true,
		},
		{
			name:      "explicit true",
			autostart: boolPtr(true),
			expected:  true,
		},
		{
			name:      "explicit false",
			autostart: boolPtr(false),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{
				Spec: VirtualMachineSpec{
					Autostart: tt.autostart,
				},
			}
			if got := vm.IsAutostart(); got != tt.expected {
				t.Errorf("Expected IsAutostart() = %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGetCPUMode(t *testing.T) {
	tests := []struct {
		name     string
		cpuMode  string
		expected string
	}{
		{
			name:     "empty defaults to host-model",
			cpuMode:  "",
			expected: "host-model",
		},
		{
			name:     "host-model",
			cpuMode:  "host-model",
			expected: "host-model",
		},
		{
			name:     "host-passthrough",
			cpuMode:  "host-passthrough",
			expected: "host-passthrough",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{
				Spec: VirtualMachineSpec{
					CPUMode: tt.cpuMode,
				},
			}
			if got := vm.GetCPUMode(); got != tt.expected {
				t.Errorf("Expected GetCPUMode() = %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetStoragePool(t *testing.T) {
	tests := []struct {
		name     string
		pool     string
		expected string
	}{
		{
			name:     "empty defaults to foundry-vms",
			pool:     "",
			expected: "foundry-vms",
		},
		{
			name:     "custom pool",
			pool:     "my-custom-pool",
			expected: "my-custom-pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{
				Spec: VirtualMachineSpec{
					StoragePool: tt.pool,
				},
			}
			if got := vm.GetStoragePool(); got != tt.expected {
				t.Errorf("Expected GetStoragePool() = %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetBootDiskFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "empty defaults to qcow2",
			format:   "",
			expected: "qcow2",
		},
		{
			name:     "qcow2",
			format:   "qcow2",
			expected: "qcow2",
		},
		{
			name:     "raw",
			format:   "raw",
			expected: "raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{
				Spec: VirtualMachineSpec{
					BootDisk: BootDiskSpec{
						Format: tt.format,
					},
				},
			}
			if got := vm.GetBootDiskFormat(); got != tt.expected {
				t.Errorf("Expected GetBootDiskFormat() = %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetBootDiskImagePool(t *testing.T) {
	tests := []struct {
		name     string
		pool     string
		expected string
	}{
		{
			name:     "empty defaults to foundry-images",
			pool:     "",
			expected: "foundry-images",
		},
		{
			name:     "custom pool",
			pool:     "my-images",
			expected: "my-images",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{
				Spec: VirtualMachineSpec{
					BootDisk: BootDiskSpec{
						ImagePool: tt.pool,
					},
				},
			}
			if got := vm.GetBootDiskImagePool(); got != tt.expected {
				t.Errorf("Expected GetBootDiskImagePool() = %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetName(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Name: "test-vm",
		},
	}
	if got := vm.GetName(); got != "test-vm" {
		t.Errorf("Expected GetName() = test-vm, got %s", got)
	}
}

func TestPhaseGettersSetters(t *testing.T) {
	vm := &VirtualMachine{}

	// Test SetPhase and GetPhase
	vm.SetPhase(VMPhaseRunning)
	if got := vm.GetPhase(); got != VMPhaseRunning {
		t.Errorf("Expected phase Running, got %s", got)
	}

	vm.SetPhase(VMPhaseStopped)
	if got := vm.GetPhase(); got != VMPhaseStopped {
		t.Errorf("Expected phase Stopped, got %s", got)
	}
}

func TestDomainUUIDGettersSetters(t *testing.T) {
	vm := &VirtualMachine{}

	testUUID := "12345678-1234-1234-1234-123456789abc"
	vm.SetDomainUUID(testUUID)
	if got := vm.GetDomainUUID(); got != testUUID {
		t.Errorf("Expected domain UUID %s, got %s", testUUID, got)
	}
}

func TestAddAddress(t *testing.T) {
	vm := &VirtualMachine{}

	vm.AddAddress("InternalIP", "10.0.0.1")
	if len(vm.Status.Addresses) != 1 {
		t.Fatalf("Expected 1 address, got %d", len(vm.Status.Addresses))
	}
	if vm.Status.Addresses[0].Type != "InternalIP" {
		t.Errorf("Expected type InternalIP, got %s", vm.Status.Addresses[0].Type)
	}
	if vm.Status.Addresses[0].Address != "10.0.0.1" {
		t.Errorf("Expected address 10.0.0.1, got %s", vm.Status.Addresses[0].Address)
	}

	vm.AddAddress("ExternalIP", "203.0.113.1")
	if len(vm.Status.Addresses) != 2 {
		t.Fatalf("Expected 2 addresses, got %d", len(vm.Status.Addresses))
	}
}

func TestMACAddressesGettersSetters(t *testing.T) {
	vm := &VirtualMachine{}

	macs := []string{"be:ef:0a:fa:fa:0a", "be:ef:0a:fa:fa:0b"}
	vm.SetMACAddresses(macs)

	got := vm.GetMACAddresses()
	if len(got) != 2 {
		t.Fatalf("Expected 2 MACs, got %d", len(got))
	}
	if got[0] != macs[0] || got[1] != macs[1] {
		t.Errorf("Expected MACs %v, got %v", macs, got)
	}
}

func TestInterfaceNamesGettersSetters(t *testing.T) {
	vm := &VirtualMachine{}

	names := []string{"vm0afafa0a", "vm0afafa0b"}
	vm.SetInterfaceNames(names)

	got := vm.GetInterfaceNames()
	if len(got) != 2 {
		t.Fatalf("Expected 2 interface names, got %d", len(got))
	}
	if got[0] != names[0] || got[1] != names[1] {
		t.Errorf("Expected interface names %v, got %v", names, got)
	}
}

func TestUpdateObservedGeneration(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Generation: 5,
		},
	}

	vm.UpdateObservedGeneration()
	if vm.Status.ObservedGeneration != 5 {
		t.Errorf("Expected ObservedGeneration 5, got %d", vm.Status.ObservedGeneration)
	}
}

func TestGetBootVolumeName(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Name: "test-vm",
		},
	}

	expected := "test-vm_boot.qcow2"
	if got := vm.GetBootVolumeName(); got != expected {
		t.Errorf("Expected boot volume name %s, got %s", expected, got)
	}
}

func TestGetDataVolumeName(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Name: "test-vm",
		},
	}

	tests := []struct {
		device   string
		expected string
	}{
		{"vdb", "test-vm_data-vdb.qcow2"},
		{"vdc", "test-vm_data-vdc.qcow2"},
		{"sdb", "test-vm_data-sdb.qcow2"},
	}

	for _, tt := range tests {
		t.Run(tt.device, func(t *testing.T) {
			if got := vm.GetDataVolumeName(tt.device); got != tt.expected {
				t.Errorf("Expected data volume name %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetCloudInitVolumeName(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Name: "test-vm",
		},
	}

	expected := "test-vm_cloudinit.iso"
	if got := vm.GetCloudInitVolumeName(); got != expected {
		t.Errorf("Expected cloud-init volume name %s, got %s", expected, got)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    *VirtualMachine
		validate func(*testing.T, *VirtualMachine)
	}{
		{
			name: "normalize VM name to lowercase",
			input: &VirtualMachine{
				ObjectMeta: ObjectMeta{
					Name: "  TEST-VM  ",
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Name != "test-vm" {
					t.Errorf("Expected name 'test-vm', got %s", vm.Name)
				}
			},
		},
		{
			name: "normalize FQDN to lowercase",
			input: &VirtualMachine{
				Spec: VirtualMachineSpec{
					CloudInit: &CloudInitSpec{
						FQDN: "  TEST.EXAMPLE.COM  ",
					},
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Spec.CloudInit.FQDN != "test.example.com" {
					t.Errorf("Expected FQDN 'test.example.com', got %s", vm.Spec.CloudInit.FQDN)
				}
			},
		},
		{
			name: "set default storage pool",
			input: &VirtualMachine{
				Spec: VirtualMachineSpec{
					StoragePool: "",
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Spec.StoragePool != "foundry-vms" {
					t.Errorf("Expected storage pool 'foundry-vms', got %s", vm.Spec.StoragePool)
				}
			},
		},
		{
			name: "set default image pool",
			input: &VirtualMachine{
				Spec: VirtualMachineSpec{
					BootDisk: BootDiskSpec{
						ImagePool: "",
					},
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Spec.BootDisk.ImagePool != "foundry-images" {
					t.Errorf("Expected image pool 'foundry-images', got %s", vm.Spec.BootDisk.ImagePool)
				}
			},
		},
		{
			name: "preserve custom pools",
			input: &VirtualMachine{
				Spec: VirtualMachineSpec{
					StoragePool: "my-pool",
					BootDisk: BootDiskSpec{
						ImagePool: "my-images",
					},
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Spec.StoragePool != "my-pool" {
					t.Errorf("Expected storage pool 'my-pool', got %s", vm.Spec.StoragePool)
				}
				if vm.Spec.BootDisk.ImagePool != "my-images" {
					t.Errorf("Expected image pool 'my-images', got %s", vm.Spec.BootDisk.ImagePool)
				}
			},
		},
		{
			name: "preserve bridge name case",
			input: &VirtualMachine{
				Spec: VirtualMachineSpec{
					NetworkInterfaces: []NetworkInterfaceSpec{
						{Bridge: "BR250"},
					},
				},
			},
			validate: func(t *testing.T, vm *VirtualMachine) {
				if vm.Spec.NetworkInterfaces[0].Bridge != "BR250" {
					t.Errorf("Expected bridge 'BR250', got %s", vm.Spec.NetworkInterfaces[0].Bridge)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Normalize()
			tt.validate(t, tt.input)
		})
	}
}

func TestNormalizeNilCloudInit(t *testing.T) {
	vm := &VirtualMachine{
		ObjectMeta: ObjectMeta{
			Name: "TEST",
		},
		Spec: VirtualMachineSpec{
			CloudInit: nil,
		},
	}

	// Should not panic
	vm.Normalize()

	if vm.Name != "test" {
		t.Errorf("Expected normalized name 'test', got %s", vm.Name)
	}
}

func TestConstants(t *testing.T) {
	if GroupName != "foundry.cofront.xyz" {
		t.Errorf("Expected GroupName 'foundry.cofront.xyz', got %s", GroupName)
	}
	if Version != "v1alpha1" {
		t.Errorf("Expected Version 'v1alpha1', got %s", Version)
	}
	if VirtualMachineKind != "VirtualMachine" {
		t.Errorf("Expected VirtualMachineKind 'VirtualMachine', got %s", VirtualMachineKind)
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}
