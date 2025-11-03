package v1alpha1

import (
	"testing"
	"time"
)

func TestVirtualMachine_DeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *VirtualMachine
	}{
		{
			name:  "nil returns nil",
			input: nil,
		},
		{
			name: "complete VM with all fields",
			input: &VirtualMachine{
				TypeMeta: TypeMeta{
					Kind:       "VirtualMachine",
					APIVersion: "foundry.cofront.xyz/v1alpha1",
				},
				ObjectMeta: ObjectMeta{
					Name: "test-vm",
					Labels: map[string]string{
						"app": "web",
					},
					Annotations: map[string]string{
						"note": "test",
					},
					UID:               "12345",
					Generation:        1,
					CreationTimestamp: Time{Time: time.Now()},
				},
				Spec: VirtualMachineSpec{
					VCPUs:       2,
					MemoryGiB:   4,
					CPUMode:     "host-model",
					StoragePool: "foundry-vms",
					BootDisk: BootDiskSpec{
						SizeGB:    50,
						Image:     "fedora-43.qcow2",
						ImagePool: "foundry-images",
						Format:    "qcow2",
					},
					DataDisks: []DataDiskSpec{
						{Device: "vdb", SizeGB: 100},
					},
					NetworkInterfaces: []NetworkInterfaceSpec{
						{
							IP:           "10.0.0.1/24",
							Gateway:      "10.0.0.254",
							Bridge:       "br0",
							DNSServers:   []string{"8.8.8.8"},
							DefaultRoute: true,
						},
					},
					CloudInit: &CloudInitSpec{
						FQDN:              "test.example.com",
						SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA..."},
					},
					Autostart: boolPtr(true),
				},
				Status: VirtualMachineStatus{
					Phase:      VMPhaseRunning,
					DomainUUID: "uuid-123",
					Addresses: []VMAddress{
						{Type: "InternalIP", Address: "10.0.0.1"},
					},
					MACAddresses:       []string{"be:ef:00:00:00:01"},
					InterfaceNames:     []string{"vm00000001"},
					ObservedGeneration: 1,
					Conditions: []Condition{
						{
							Type:   "Ready",
							Status: ConditionTrue,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := tt.input.DeepCopy()

			if tt.input == nil {
				if copy != nil {
					t.Error("DeepCopy() of nil should return nil")
				}
				return
			}

			if copy == nil {
				t.Fatal("DeepCopy() returned nil for non-nil input")
			}

			// Verify basic fields
			if copy.Name != tt.input.Name {
				t.Errorf("Name mismatch")
			}
			if copy.Spec.VCPUs != tt.input.Spec.VCPUs {
				t.Errorf("VCPUs mismatch")
			}

			// Verify slice independence - modify copy
			if len(tt.input.Spec.DataDisks) > 0 {
				copy.Spec.DataDisks[0].SizeGB = 9999
				if tt.input.Spec.DataDisks[0].SizeGB == 9999 {
					t.Error("Modifying copy.Spec.DataDisks affected original")
				}
			}

			// Verify map independence
			if tt.input.Labels != nil {
				copy.Labels["new"] = "label"
				if _, exists := tt.input.Labels["new"]; exists {
					t.Error("Modifying copy.Labels affected original")
				}
			}

			// Verify nested struct independence
			copy.Spec.BootDisk.SizeGB = 9999
			if tt.input.Spec.BootDisk.SizeGB == 9999 {
				t.Error("Modifying copy.Spec.BootDisk affected original")
			}

			// Verify pointer independence
			if tt.input.Spec.CloudInit != nil {
				copy.Spec.CloudInit.FQDN = "modified"
				if tt.input.Spec.CloudInit.FQDN == "modified" {
					t.Error("Modifying copy.Spec.CloudInit affected original")
				}
			}

			// Verify status slice independence
			if len(tt.input.Status.Addresses) > 0 {
				copy.Status.Addresses[0].Address = "modified"
				if tt.input.Status.Addresses[0].Address == "modified" {
					t.Error("Modifying copy.Status.Addresses affected original")
				}
			}
		})
	}
}

func TestVirtualMachineSpec_DeepCopy(t *testing.T) {
	autostart := true
	spec := &VirtualMachineSpec{
		VCPUs:     2,
		MemoryGiB: 4,
		DataDisks: []DataDiskSpec{
			{Device: "vdb", SizeGB: 100},
		},
		NetworkInterfaces: []NetworkInterfaceSpec{
			{IP: "10.0.0.1/24", Bridge: "br0"},
		},
		CloudInit: &CloudInitSpec{
			FQDN: "test.example.com",
		},
		Autostart: &autostart,
	}

	copy := spec.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	// Verify independence
	copy.VCPUs = 99
	if spec.VCPUs == 99 {
		t.Error("Modifying copy affected original")
	}

	copy.DataDisks[0].SizeGB = 999
	if spec.DataDisks[0].SizeGB == 999 {
		t.Error("Modifying copy.DataDisks affected original")
	}

	copy.CloudInit.FQDN = "modified"
	if spec.CloudInit.FQDN == "modified" {
		t.Error("Modifying copy.CloudInit affected original")
	}

	*copy.Autostart = false
	if *spec.Autostart == false {
		t.Error("Modifying copy.Autostart affected original")
	}
}

func TestVirtualMachineSpec_DeepCopy_NilPointers(t *testing.T) {
	spec := &VirtualMachineSpec{
		VCPUs:     1,
		MemoryGiB: 2,
		CloudInit: nil,
		Autostart: nil,
	}

	copy := spec.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	if copy.CloudInit != nil {
		t.Error("Expected CloudInit to be nil")
	}

	if copy.Autostart != nil {
		t.Error("Expected Autostart to be nil")
	}
}

func TestBootDiskSpec_DeepCopy(t *testing.T) {
	disk := &BootDiskSpec{
		SizeGB:    50,
		Image:     "fedora.qcow2",
		ImagePool: "foundry-images",
		Format:    "qcow2",
	}

	copy := disk.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	// Verify independence
	copy.SizeGB = 999
	if disk.SizeGB == 999 {
		t.Error("Modifying copy affected original")
	}
}

func TestDataDiskSpec_DeepCopy(t *testing.T) {
	disk := &DataDiskSpec{
		Device: "vdb",
		SizeGB: 100,
	}

	copy := disk.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	copy.SizeGB = 999
	if disk.SizeGB == 999 {
		t.Error("Modifying copy affected original")
	}
}

func TestNetworkInterfaceSpec_DeepCopy(t *testing.T) {
	iface := &NetworkInterfaceSpec{
		IP:           "10.0.0.1/24",
		Gateway:      "10.0.0.254",
		Bridge:       "br0",
		DNSServers:   []string{"8.8.8.8", "1.1.1.1"},
		DefaultRoute: true,
	}

	copy := iface.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	// Verify slice independence
	copy.DNSServers[0] = "modified"
	if iface.DNSServers[0] == "modified" {
		t.Error("Modifying copy.DNSServers affected original")
	}

	copy.Bridge = "modified"
	if iface.Bridge == "modified" {
		t.Error("Modifying copy affected original")
	}
}

func TestCloudInitSpec_DeepCopy(t *testing.T) {
	ci := &CloudInitSpec{
		FQDN:              "test.example.com",
		SSHAuthorizedKeys: []string{"key1", "key2"},
		PasswordHash:      "$6$...",
		SSHPasswordAuth:   true,
	}

	copy := ci.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	// Verify slice independence
	copy.SSHAuthorizedKeys[0] = "modified"
	if ci.SSHAuthorizedKeys[0] == "modified" {
		t.Error("Modifying copy.SSHAuthorizedKeys affected original")
	}

	copy.FQDN = "modified"
	if ci.FQDN == "modified" {
		t.Error("Modifying copy affected original")
	}
}

func TestVirtualMachineStatus_DeepCopy(t *testing.T) {
	status := &VirtualMachineStatus{
		Phase:      VMPhaseRunning,
		DomainUUID: "uuid-123",
		Addresses: []VMAddress{
			{Type: "InternalIP", Address: "10.0.0.1"},
			{Type: "ExternalIP", Address: "203.0.113.1"},
		},
		MACAddresses:   []string{"be:ef:00:00:00:01"},
		InterfaceNames: []string{"vm00000001"},
		Conditions: []Condition{
			{Type: "Ready", Status: ConditionTrue},
		},
		ObservedGeneration: 5,
	}

	copy := status.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	// Verify slice independence
	copy.Addresses[0].Address = "modified"
	if status.Addresses[0].Address == "modified" {
		t.Error("Modifying copy.Addresses affected original")
	}

	copy.MACAddresses[0] = "modified"
	if status.MACAddresses[0] == "modified" {
		t.Error("Modifying copy.MACAddresses affected original")
	}

	copy.Conditions[0].Status = ConditionFalse
	if status.Conditions[0].Status == ConditionFalse {
		t.Error("Modifying copy.Conditions affected original")
	}
}

func TestVMAddress_DeepCopy(t *testing.T) {
	addr := &VMAddress{
		Type:    "InternalIP",
		Address: "10.0.0.1",
	}

	copy := addr.DeepCopy()

	if copy == nil {
		t.Fatal("DeepCopy() returned nil")
	}

	copy.Address = "modified"
	if addr.Address == "modified" {
		t.Error("Modifying copy affected original")
	}
}

func TestVMPhase_Constants(t *testing.T) {
	phases := map[VMPhase]string{
		VMPhasePending:  "Pending",
		VMPhaseCreating: "Creating",
		VMPhaseRunning:  "Running",
		VMPhaseStopping: "Stopping",
		VMPhaseStopped:  "Stopped",
		VMPhaseFailed:   "Failed",
	}

	for phase, expected := range phases {
		if string(phase) != expected {
			t.Errorf("Phase constant mismatch: got %s, want %s", phase, expected)
		}
	}
}

func TestConditionConstants(t *testing.T) {
	conditions := map[string]string{
		ConditionReady:              "Ready",
		ConditionStorageProvisioned: "StorageProvisioned",
		ConditionNetworkConfigured:  "NetworkConfigured",
		ConditionCloudInitReady:     "CloudInitReady",
	}

	for constant, expected := range conditions {
		if constant != expected {
			t.Errorf("Condition constant mismatch: got %s, want %s", constant, expected)
		}
	}
}

// Note: VirtualMachineList tests will be added when the type is implemented
