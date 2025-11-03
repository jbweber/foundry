package cloudinit

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/kdomanski/iso9660"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestGenerateISO(t *testing.T) {
	tests := []struct {
		name    string
		vm      *v1alpha1.VirtualMachine
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with all fields",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "test-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/fedora.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.20.30.40/24",
							Gateway:      "10.20.30.1",
							DNSServers:   []string{"8.8.8.8", "1.1.1.1"},
							Bridge:       "br0",
							DefaultRoute: true,
						},
					},
					CloudInit: &v1alpha1.CloudInitSpec{
						FQDN:              "test-vm.example.com",
						SSHAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo test@example.com"},
						PasswordHash:      "$6$rounds=4096$salt$hash",
						SSHPasswordAuth:   false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with minimal fields",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "minimal-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     1,
					MemoryGiB: 2,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 10,
						Image:  "/var/lib/libvirt/images/base.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "192.168.1.100/24",
							Gateway:      "192.168.1.1",
							Bridge:       "virbr0",
							DefaultRoute: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with multiple interfaces",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "multi-nic-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     4,
					MemoryGiB: 8,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "/var/lib/libvirt/images/ubuntu.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.0.1.10/24",
							Gateway:      "10.0.1.1",
							DNSServers:   []string{"8.8.8.8"},
							Bridge:       "br0",
							DefaultRoute: true,
						},
						{
							IP:           "10.0.2.10/24",
							Gateway:      "10.0.2.1",
							DNSServers:   []string{"8.8.4.4"},
							Bridge:       "br1",
							DefaultRoute: false,
						},
					},
					CloudInit: &v1alpha1.CloudInitSpec{
						FQDN:              "multi-nic-vm.local",
						SSHAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ test@host"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			vm:      nil,
			wantErr: true,
			errMsg:  "VM configuration cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isoBytes, err := GenerateISO(tt.vm)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateISO() expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("GenerateISO() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			// No error expected
			if err != nil {
				t.Fatalf("GenerateISO() unexpected error: %v", err)
			}

			if len(isoBytes) == 0 {
				t.Fatal("GenerateISO() returned empty byte slice")
			}

			// Verify the ISO structure
			verifyISOStructure(t, isoBytes, tt.vm)
		})
	}
}

func TestGenerateISO_ErrorPropagation(t *testing.T) {
	tests := []struct {
		name      string
		vm        *v1alpha1.VirtualMachine
		wantErr   bool
		errSubstr string // Substring that should appear in error
	}{
		{
			name: "error from GenerateUserData - nil cloud-init with password",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "test-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.0.1.10/24",
							Gateway:      "10.0.1.1",
							Bridge:       "br0",
							DefaultRoute: true,
						},
					},
				},
			},
			wantErr: false, // This should succeed - just testing the path
		},
		{
			name: "error from GenerateNetworkConfig - no interfaces",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "test-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:             2,
					MemoryGiB:         4,
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{},
				},
			},
			wantErr:   true,
			errSubstr: "failed to generate network-config",
		},
		{
			name: "error from GenerateNetworkConfig - invalid IP",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "test-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "invalid-ip",
							Gateway:      "10.0.1.1",
							Bridge:       "br0",
							DefaultRoute: true,
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "failed to generate network-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateISO(tt.vm)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateISO() expected error but got nil")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("GenerateISO() error = %v, want error containing %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("GenerateISO() unexpected error: %v", err)
				}
			}
		})
	}
}

// verifyISOStructure reads the generated ISO and verifies its contents
func verifyISOStructure(t *testing.T, isoBytes []byte, vm *v1alpha1.VirtualMachine) {
	t.Helper()

	// Create a reader from the ISO bytes
	reader := bytes.NewReader(isoBytes)

	// Open the ISO image
	img, err := iso9660.OpenImage(reader)
	if err != nil {
		t.Fatalf("failed to open ISO image: %v", err)
	}

	// Verify volume identifier using Label() method
	volumeID, err := img.Label()
	if err != nil {
		t.Fatalf("failed to get volume label: %v", err)
	}
	expectedVolumeID := "CIDATA"
	if volumeID != expectedVolumeID {
		t.Errorf("ISO volume identifier = %q, want %q", volumeID, expectedVolumeID)
	}

	// Get the root directory
	rootDir, err := img.RootDir()
	if err != nil {
		t.Fatalf("failed to get root directory: %v", err)
	}

	// Get children from root directory
	children, err := rootDir.GetChildren()
	if err != nil {
		t.Fatalf("failed to get children: %v", err)
	}

	// Verify the three required files exist
	requiredFiles := []string{"user-data", "meta-data", "network-config"}
	for _, filename := range requiredFiles {
		found := false
		for _, child := range children {
			if child.Name() == filename {
				found = true

				// Read and verify file content
				content, err := readISOFile(child)
				if err != nil {
					t.Errorf("failed to read %s: %v", filename, err)
					continue
				}

				// Verify content matches what generators would produce
				var expected string
				switch filename {
				case "user-data":
					expected, err = GenerateUserData(vm)
				case "meta-data":
					expected, err = GenerateMetaData(vm)
				case "network-config":
					expected, err = GenerateNetworkConfig(vm)
				}

				if err != nil {
					t.Errorf("failed to generate expected %s: %v", filename, err)
					continue
				}

				if content != expected {
					t.Errorf("%s content mismatch:\ngot:\n%s\n\nwant:\n%s", filename, content, expected)
				}

				break
			}
		}

		if !found {
			t.Errorf("required file %q not found in ISO", filename)
		}
	}

	// Verify we have exactly 3 files (no extra files)
	if len(children) != 3 {
		t.Errorf("ISO contains %d files, want 3", len(children))
	}
}

// readISOFile reads the content of a file from the ISO image
func readISOFile(file *iso9660.File) (string, error) {
	reader := file.Reader()
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func TestGenerateISO_VolumeIDFormat(t *testing.T) {
	// Test that volume ID is exactly "CIDATA" (uppercase, no truncation)
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "vol-test",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     1,
			MemoryGiB: 1,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 10,
				Image:  "/var/lib/libvirt/images/test.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:           "10.0.0.1/24",
					Gateway:      "10.0.0.254",
					Bridge:       "br0",
					DefaultRoute: true,
				},
			},
		},
	}

	isoBytes, err := GenerateISO(vm)
	if err != nil {
		t.Fatalf("GenerateISO() error: %v", err)
	}

	reader := bytes.NewReader(isoBytes)
	img, err := iso9660.OpenImage(reader)
	if err != nil {
		t.Fatalf("failed to open ISO: %v", err)
	}

	volumeID, err := img.Label()
	if err != nil {
		t.Fatalf("failed to get volume label: %v", err)
	}
	if volumeID != "CIDATA" {
		t.Errorf("volume ID = %q, want %q (must be uppercase CIDATA)", volumeID, "CIDATA")
	}
}

func TestGenerateISO_FileNamesExact(t *testing.T) {
	// Test that file names are exactly as cloud-init expects (no extensions, exact case)
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "filename-test",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     1,
			MemoryGiB: 1,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 10,
				Image:  "/var/lib/libvirt/images/test.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:           "10.0.0.1/24",
					Gateway:      "10.0.0.254",
					Bridge:       "br0",
					DefaultRoute: true,
				},
			},
		},
	}

	isoBytes, err := GenerateISO(vm)
	if err != nil {
		t.Fatalf("GenerateISO() error: %v", err)
	}

	reader := bytes.NewReader(isoBytes)
	img, err := iso9660.OpenImage(reader)
	if err != nil {
		t.Fatalf("failed to open ISO: %v", err)
	}

	rootDir, err := img.RootDir()
	if err != nil {
		t.Fatalf("failed to get root dir: %v", err)
	}

	children, err := rootDir.GetChildren()
	if err != nil {
		t.Fatalf("failed to get children: %v", err)
	}

	// Verify exact filenames (case-sensitive, no extensions)
	expectedNames := map[string]bool{
		"user-data":      false,
		"meta-data":      false,
		"network-config": false,
	}

	for _, child := range children {
		name := child.Name()
		if _, ok := expectedNames[name]; ok {
			expectedNames[name] = true
		} else {
			t.Errorf("unexpected file in ISO: %q", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("required file %q not found in ISO", name)
		}
	}
}
