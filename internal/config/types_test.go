package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile_ValidConfig(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-vm.yaml")

	configYAML := `name: test-vm
vcpus: 4
memory_gib: 8
boot_disk:
  size_gb: 50
  image: /var/lib/libvirt/images/base/fedora-42.qcow2
network_interfaces:
  - ip: 10.20.30.40/24
    gateway: 10.20.30.1
    dns_servers:
      - 8.8.8.8
      - 1.1.1.1
    bridge: br0
cloud_init:
  fqdn: test-vm.example.com
  ssh_keys:
    - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIbJKZscbOLzBsgY5y2QupKW4A2kSDjMBQGPb1dChr+S test@example.com
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify basic fields
	if config.Name != "test-vm" {
		t.Errorf("Expected name 'test-vm', got %q", config.Name)
	}
	if config.VCPUs != 4 {
		t.Errorf("Expected 4 vcpus, got %d", config.VCPUs)
	}
	if config.MemoryGiB != 8 {
		t.Errorf("Expected 8 GiB memory, got %d", config.MemoryGiB)
	}

	// Verify boot disk
	if config.BootDisk.SizeGB != 50 {
		t.Errorf("Expected boot disk size 50 GB, got %d", config.BootDisk.SizeGB)
	}
	if config.BootDisk.Image != "/var/lib/libvirt/images/base/fedora-42.qcow2" {
		t.Errorf("Expected image path, got %q", config.BootDisk.Image)
	}

	// Verify network interface
	if len(config.Network) != 1 {
		t.Fatalf("Expected 1 network interface, got %d", len(config.Network))
	}
	iface := config.Network[0]
	if iface.IP != "10.20.30.40/24" {
		t.Errorf("Expected IP '10.20.30.40/24', got %q", iface.IP)
	}
	if iface.Gateway != "10.20.30.1" {
		t.Errorf("Expected gateway '10.20.30.1', got %q", iface.Gateway)
	}
	if iface.Bridge != "br0" {
		t.Errorf("Expected bridge 'br0', got %q", iface.Bridge)
	}
	if len(iface.DNSServers) != 2 {
		t.Errorf("Expected 2 DNS servers, got %d", len(iface.DNSServers))
	}

	// Verify MAC address was calculated from IP
	expectedMAC := "be:ef:0a:14:1e:28" // From IP 10.20.30.40
	if iface.MACAddress != expectedMAC {
		t.Errorf("Expected MAC address %q, got %q", expectedMAC, iface.MACAddress)
	}

	// Verify cloud-init
	if config.CloudInit == nil {
		t.Fatal("Expected cloud_init config, got nil")
	}
	if config.CloudInit.FQDN != "test-vm.example.com" {
		t.Errorf("Expected FQDN 'test-vm.example.com', got %q", config.CloudInit.FQDN)
	}
	if len(config.CloudInit.SSHKeys) != 1 {
		t.Errorf("Expected 1 SSH key, got %d", len(config.CloudInit.SSHKeys))
	}
}

func TestLoadFromFile_MultiDiskConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "multi-disk.yaml")

	configYAML := `name: multi-disk-vm
vcpus: 2
memory_gib: 4
boot_disk:
  size_gb: 30
  image: /var/lib/libvirt/images/base/fedora.qcow2
data_disks:
  - device: vdb
    size_gb: 100
  - device: vdc
    size_gb: 200
network_interfaces:
  - ip: 192.168.1.50/24
    gateway: 192.168.1.1
    dns_servers:
      - 192.168.1.1
    bridge: br0
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify data disks
	if len(config.DataDisks) != 2 {
		t.Fatalf("Expected 2 data disks, got %d", len(config.DataDisks))
	}
	if config.DataDisks[0].Device != "vdb" {
		t.Errorf("Expected device 'vdb', got %q", config.DataDisks[0].Device)
	}
	if config.DataDisks[0].SizeGB != 100 {
		t.Errorf("Expected size 100 GB, got %d", config.DataDisks[0].SizeGB)
	}
	if config.DataDisks[1].Device != "vdc" {
		t.Errorf("Expected device 'vdc', got %q", config.DataDisks[1].Device)
	}
	if config.DataDisks[1].SizeGB != 200 {
		t.Errorf("Expected size 200 GB, got %d", config.DataDisks[1].SizeGB)
	}
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		config    VMConfig
		expectErr string
	}{
		{
			name:      "missing name",
			config:    VMConfig{VCPUs: 2, MemoryGiB: 4},
			expectErr: "name is required",
		},
		{
			name:      "zero vcpus",
			config:    VMConfig{Name: "test", VCPUs: 0, MemoryGiB: 4},
			expectErr: "vcpus must be > 0, got 0",
		},
		{
			name:      "zero memory",
			config:    VMConfig{Name: "test", VCPUs: 2, MemoryGiB: 0},
			expectErr: "memory_gib must be > 0, got 0",
		},
		{
			name: "no network interfaces",
			config: VMConfig{
				Name:      "test",
				VCPUs:     2,
				MemoryGiB: 4,
				BootDisk:  BootDiskConfig{SizeGB: 50, Image: "/path/to/image.qcow2"},
			},
			expectErr: "at least one network_interfaces entry is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}
			if err.Error() != tt.expectErr {
				t.Errorf("Expected error %q, got %q", tt.expectErr, err.Error())
			}
		})
	}
}

func TestBootDiskValidate(t *testing.T) {
	tests := []struct {
		name      string
		disk      BootDiskConfig
		expectErr string
	}{
		{
			name:      "zero size",
			disk:      BootDiskConfig{SizeGB: 0, Image: "/path/to/image"},
			expectErr: "size_gb must be > 0, got 0",
		},
		{
			name:      "no image and not empty",
			disk:      BootDiskConfig{SizeGB: 50},
			expectErr: "must specify either 'image' or 'empty: true'",
		},
		{
			name:      "both image and empty",
			disk:      BootDiskConfig{SizeGB: 50, Image: "/path", Empty: true},
			expectErr: "cannot specify both 'image' and 'empty: true'",
		},
		{
			name: "valid with image",
			disk: BootDiskConfig{SizeGB: 50, Image: "/path/to/image.qcow2"},
		},
		{
			name: "valid empty",
			disk: BootDiskConfig{SizeGB: 50, Empty: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.disk.Validate()
			if tt.expectErr == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("Expected validation error, got nil")
				}
				if err.Error() != tt.expectErr {
					t.Errorf("Expected error %q, got %q", tt.expectErr, err.Error())
				}
			}
		})
	}
}

func TestNetworkInterfaceValidate(t *testing.T) {
	tests := []struct {
		name      string
		iface     NetworkInterface
		expectErr string
	}{
		{
			name:      "missing IP",
			iface:     NetworkInterface{Gateway: "10.0.0.1", Bridge: "br0"},
			expectErr: "ip is required",
		},
		{
			name:      "invalid IP format",
			iface:     NetworkInterface{IP: "not-an-ip", Gateway: "10.0.0.1", Bridge: "br0"},
			expectErr: "invalid ip/cidr format",
		},
		{
			name:      "missing CIDR",
			iface:     NetworkInterface{IP: "10.0.0.50", Gateway: "10.0.0.1", Bridge: "br0"},
			expectErr: "invalid ip/cidr format",
		},
		{
			name:      "invalid gateway",
			iface:     NetworkInterface{IP: "10.0.0.50/24", Gateway: "not-an-ip", Bridge: "br0"},
			expectErr: "invalid gateway IP address",
		},
		{
			name:      "invalid DNS server",
			iface:     NetworkInterface{IP: "10.0.0.50/24", Gateway: "10.0.0.1", DNSServers: []string{"not-an-ip"}, Bridge: "br0"},
			expectErr: "dns_servers[0] is not a valid IP address",
		},
		{
			name: "valid interface",
			iface: NetworkInterface{
				IP:         "10.0.0.50/24",
				Gateway:    "10.0.0.1",
				DNSServers: []string{"8.8.8.8", "1.1.1.1"},
				Bridge:     "br0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.iface.Validate()
			if tt.expectErr == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("Expected validation error, got nil")
				}
				// Check if error contains expected string (not exact match due to wrapping)
				if len(err.Error()) < len(tt.expectErr) || err.Error()[:len(tt.expectErr)] != tt.expectErr {
					t.Errorf("Expected error starting with %q, got %q", tt.expectErr, err.Error())
				}
			}
		})
	}
}

func TestCloudInitValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    CloudInitConfig
		expectErr string
	}{
		{
			name: "invalid SSH key - too short",
			config: CloudInitConfig{
				SSHKeys: []string{"short"},
			},
			expectErr: "ssh_keys[0] is not a valid SSH public key",
		},
		{
			name: "invalid SSH key - wrong prefix",
			config: CloudInitConfig{
				SSHKeys: []string{"not-a-valid-ssh-key-but-long-enough"},
			},
			expectErr: "ssh_keys[0] is not a valid SSH public key",
		},
		{
			name: "invalid SSH key - invalid base64",
			config: CloudInitConfig{
				SSHKeys: []string{"ssh-ed25519 not-valid-base64!!!"},
			},
			expectErr: "ssh_keys[0] is not a valid SSH public key",
		},
		{
			name: "invalid password hash",
			config: CloudInitConfig{
				RootPasswordHash: "plaintext",
			},
			expectErr: "root_password_hash must be a valid crypt hash",
		},
		{
			name: "invalid FQDN - no dot",
			config: CloudInitConfig{
				FQDN: "hostname",
			},
			expectErr: "fqdn must be a valid hostname with domain",
		},
		{
			name: "invalid FQDN - starts with hyphen",
			config: CloudInitConfig{
				FQDN: "-bad.example.com",
			},
			expectErr: "fqdn must be a valid hostname with domain",
		},
		{
			name: "invalid FQDN - ends with hyphen",
			config: CloudInitConfig{
				FQDN: "bad-.example.com",
			},
			expectErr: "fqdn must be a valid hostname with domain",
		},
		{
			name: "invalid FQDN - uppercase (should be normalized first)",
			config: CloudInitConfig{
				FQDN: "Host.Example.COM",
			},
			expectErr: "fqdn must be a valid hostname with domain",
		},
		{
			name: "invalid FQDN - contains underscore",
			config: CloudInitConfig{
				FQDN: "host_name.example.com",
			},
			expectErr: "fqdn must be a valid hostname with domain",
		},
		{
			name: "valid FQDN - simple",
			config: CloudInitConfig{
				FQDN: "host.example.com",
			},
		},
		{
			name: "valid FQDN - with hyphens",
			config: CloudInitConfig{
				FQDN: "my-host.my-domain.com",
			},
		},
		{
			name: "valid FQDN - subdomain",
			config: CloudInitConfig{
				FQDN: "web01.prod.example.com",
			},
		},
		{
			name: "valid config - ed25519 key",
			config: CloudInitConfig{
				FQDN:             "test-vm.example.com",
				SSHKeys:          []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIbJKZscbOLzBsgY5y2QupKW4A2kSDjMBQGPb1dChr+S test@example.com"},
				RootPasswordHash: "$6$rounds=4096$salt$hashedpassword",
			},
		},
		{
			name: "valid config - rsa key",
			config: CloudInitConfig{
				FQDN:    "web.example.com",
				SSHKeys: []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCq7mGKPGMc36QAe7g1dJ8oGeDD1VnfBwdC3YAlp8zX3cQm8PEaaBUsKgVPigiFVWMwKTBpP2YWAjQaqyBIgFM7sneE8Ke3ouMS9GaOoFHMcorvX1N6oJtldL58D1vfGpHcBfwZiSFHxHZOZwG0Q0hCBJcoAiVtBUaubspLiXY/QgUZnw1JgbAsVuFdHxMsqSwi8NC6smVhg00T28TDubfgMZM02Uvd/qNZF6PzKxUhcCIY4zCHtsiMeN7njssKmjnuBLBlD51D19Rw6CbHsKOEskdpIHU+8o5debIwHk7c6Q0iOGTs/2lg/Rjzs+Us59NOTRB+jECEAbO0r19l//pr test-rsa@example.com"},
			},
		},
		{
			name: "valid config - multiple keys",
			config: CloudInitConfig{
				FQDN: "multi.example.com",
				SSHKeys: []string{
					"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIbJKZscbOLzBsgY5y2QupKW4A2kSDjMBQGPb1dChr+S test@example.com",
					"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCq7mGKPGMc36QAe7g1dJ8oGeDD1VnfBwdC3YAlp8zX3cQm8PEaaBUsKgVPigiFVWMwKTBpP2YWAjQaqyBIgFM7sneE8Ke3ouMS9GaOoFHMcorvX1N6oJtldL58D1vfGpHcBfwZiSFHxHZOZwG0Q0hCBJcoAiVtBUaubspLiXY/QgUZnw1JgbAsVuFdHxMsqSwi8NC6smVhg00T28TDubfgMZM02Uvd/qNZF6PzKxUhcCIY4zCHtsiMeN7njssKmjnuBLBlD51D19Rw6CbHsKOEskdpIHU+8o5debIwHk7c6Q0iOGTs/2lg/Rjzs+Us59NOTRB+jECEAbO0r19l//pr test-rsa@example.com",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("Expected validation error, got nil")
				}
				if len(err.Error()) < len(tt.expectErr) || err.Error()[:len(tt.expectErr)] != tt.expectErr {
					t.Errorf("Expected error starting with %q, got %q", tt.expectErr, err.Error())
				}
			}
		})
	}
}

func TestValidate_DuplicateChecks(t *testing.T) {
	t.Run("duplicate data disk devices", func(t *testing.T) {
		config := VMConfig{
			Name:      "test",
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk:  BootDiskConfig{SizeGB: 50, Image: "/path/to/image"},
			DataDisks: []DataDiskConfig{
				{Device: "vdb", SizeGB: 100},
				{Device: "vdb", SizeGB: 200},
			},
			Network: []NetworkInterface{
				{IP: "10.0.0.50/24", Gateway: "10.0.0.1", Bridge: "br0"},
			},
		}

		err := config.Validate()
		if err == nil {
			t.Fatal("Expected validation error for duplicate devices, got nil")
		}
		if err.Error() != "data_disks[1]: duplicate device name \"vdb\"" {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("duplicate IPs", func(t *testing.T) {
		config := VMConfig{
			Name:      "test",
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk:  BootDiskConfig{SizeGB: 50, Image: "/path/to/image"},
			Network: []NetworkInterface{
				{IP: "10.0.0.50/24", Gateway: "10.0.0.1", Bridge: "br0"},
				{IP: "10.0.0.50/24", Gateway: "10.0.0.1", Bridge: "br1"},
			},
		}

		err := config.Validate()
		if err == nil {
			t.Fatal("Expected validation error for duplicate IPs, got nil")
		}
		if err.Error() != "network_interfaces[1]: duplicate IP \"10.0.0.50/24\"" {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

// Disk Path Helper Tests

func TestDiskPathHelpers(t *testing.T) {
	config := &VMConfig{
		Name: "test-vm",
	}

	t.Run("GetBootDiskPath", func(t *testing.T) {
		expected := "test-vm/boot.qcow2"
		actual := config.GetBootDiskPath()
		if actual != expected {
			t.Errorf("Expected %q, got %q", expected, actual)
		}
	})

	t.Run("GetDataDiskPath", func(t *testing.T) {
		tests := []struct {
			device   string
			expected string
		}{
			{device: "vdb", expected: "test-vm/data-vdb.qcow2"},
			{device: "vdc", expected: "test-vm/data-vdc.qcow2"},
			{device: "vdd", expected: "test-vm/data-vdd.qcow2"},
		}

		for _, tt := range tests {
			t.Run(tt.device, func(t *testing.T) {
				actual := config.GetDataDiskPath(tt.device)
				if actual != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, actual)
				}
			})
		}
	})

	t.Run("GetCloudInitISOPath", func(t *testing.T) {
		expected := "test-vm/cloudinit.iso"
		actual := config.GetCloudInitISOPath()
		if actual != expected {
			t.Errorf("Expected %q, got %q", expected, actual)
		}
	})

	t.Run("GetVMDirectory", func(t *testing.T) {
		expected := "test-vm"
		actual := config.GetVMDirectory()
		if actual != expected {
			t.Errorf("Expected %q, got %q", expected, actual)
		}
	})

	t.Run("paths with different VM names", func(t *testing.T) {
		configs := []struct {
			name              string
			expectedBoot      string
			expectedData      string
			expectedCloudInit string
			expectedDirectory string
		}{
			{
				name:              "my-vm",
				expectedBoot:      "my-vm/boot.qcow2",
				expectedData:      "my-vm/data-vdb.qcow2",
				expectedCloudInit: "my-vm/cloudinit.iso",
				expectedDirectory: "my-vm",
			},
			{
				name:              "prod-web01",
				expectedBoot:      "prod-web01/boot.qcow2",
				expectedData:      "prod-web01/data-vdb.qcow2",
				expectedCloudInit: "prod-web01/cloudinit.iso",
				expectedDirectory: "prod-web01",
			},
			{
				name:              "a",
				expectedBoot:      "a/boot.qcow2",
				expectedData:      "a/data-vdb.qcow2",
				expectedCloudInit: "a/cloudinit.iso",
				expectedDirectory: "a",
			},
		}

		for _, tt := range configs {
			t.Run(tt.name, func(t *testing.T) {
				cfg := &VMConfig{Name: tt.name}

				if boot := cfg.GetBootDiskPath(); boot != tt.expectedBoot {
					t.Errorf("Boot path: expected %q, got %q", tt.expectedBoot, boot)
				}

				if data := cfg.GetDataDiskPath("vdb"); data != tt.expectedData {
					t.Errorf("Data path: expected %q, got %q", tt.expectedData, data)
				}

				if iso := cfg.GetCloudInitISOPath(); iso != tt.expectedCloudInit {
					t.Errorf("Cloud-init ISO path: expected %q, got %q", tt.expectedCloudInit, iso)
				}

				if dir := cfg.GetVMDirectory(); dir != tt.expectedDirectory {
					t.Errorf("VM directory: expected %q, got %q", tt.expectedDirectory, dir)
				}
			})
		}
	})
}

// MAC Calculation Tests (moved from internal/network/mac_test.go)

func TestCalculateMACFromIP(t *testing.T) {
	tests := []struct {
		name        string
		ip          string
		expectedMAC string
		expectErr   bool
	}{
		{
			name:        "simple IP with /24 CIDR",
			ip:          "10.20.30.40/24",
			expectedMAC: "be:ef:0a:14:1e:28",
		},
		{
			name:        "IP without CIDR",
			ip:          "10.20.30.40",
			expectedMAC: "be:ef:0a:14:1e:28",
		},
		{
			name:        "IP from Ansible example - 10.55.22.22",
			ip:          "10.55.22.22/24",
			expectedMAC: "be:ef:0a:37:16:16",
		},
		{
			name:        "IP with /32 CIDR",
			ip:          "192.168.1.100/32",
			expectedMAC: "be:ef:c0:a8:01:64",
		},
		{
			name:        "IP with /16 CIDR",
			ip:          "172.16.0.1/16",
			expectedMAC: "be:ef:ac:10:00:01",
		},
		{
			name:        "all zeros",
			ip:          "0.0.0.0/0",
			expectedMAC: "be:ef:00:00:00:00",
		},
		{
			name:        "all 255s",
			ip:          "255.255.255.255/32",
			expectedMAC: "be:ef:ff:ff:ff:ff",
		},
		{
			name:        "typical private IP - 192.168.1.50",
			ip:          "192.168.1.50/24",
			expectedMAC: "be:ef:c0:a8:01:32",
		},
		{
			name:      "invalid IP format",
			ip:        "not-an-ip",
			expectErr: true,
		},
		{
			name:      "invalid CIDR",
			ip:        "10.0.0.1/99",
			expectErr: true,
		},
		{
			name:      "IPv6 address",
			ip:        "2001:db8::1",
			expectErr: true,
		},
		{
			name:      "empty string",
			ip:        "",
			expectErr: true,
		},
		{
			name:      "incomplete IP",
			ip:        "10.0.0",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac, err := calculateMACFromIP(tt.ip)

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got MAC %q", tt.ip, mac)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.ip, err)
				return
			}

			if mac != tt.expectedMAC {
				t.Errorf("For IP %q, expected MAC %q, got %q", tt.ip, tt.expectedMAC, mac)
			}
		})
	}
}

// TestCalculateMACFromIP_AnsibleCompatibility verifies that our implementation
// matches the Ansible filter from homestead/roles/libvirt-create/filter_plugins/network_utils.py
func TestCalculateMACFromIP_AnsibleCompatibility(t *testing.T) {
	// Test cases derived from the Ansible implementation:
	// mac = 'be:ef:{:02x}:{:02x}:{:02x}:{:02x}'.format(*ip_parts)
	testCases := []struct {
		ip  string
		mac string
	}{
		{"10.0.0.1/24", "be:ef:0a:00:00:01"},
		{"10.0.0.2/24", "be:ef:0a:00:00:02"},
		{"10.0.0.10/24", "be:ef:0a:00:00:0a"},
		{"10.0.0.100/24", "be:ef:0a:00:00:64"},
		{"10.0.0.255/24", "be:ef:0a:00:00:ff"},
		{"192.168.1.1/24", "be:ef:c0:a8:01:01"},
		{"172.31.255.254/16", "be:ef:ac:1f:ff:fe"},
	}

	for _, tc := range testCases {
		t.Run(tc.ip, func(t *testing.T) {
			mac, err := calculateMACFromIP(tc.ip)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if mac != tc.mac {
				t.Errorf("Expected %s, got %s", tc.mac, mac)
			}
		})
	}
}

// TestCalculateMACFromIP_Uniqueness verifies that different IPs produce different MACs
func TestCalculateMACFromIP_Uniqueness(t *testing.T) {
	ips := []string{
		"10.0.0.1/24",
		"10.0.0.2/24",
		"10.0.1.1/24",
		"10.1.0.1/24",
		"11.0.0.1/24",
		"192.168.1.1/24",
		"172.16.0.1/16",
	}

	seen := make(map[string]string) // mac -> ip

	for _, ip := range ips {
		mac, err := calculateMACFromIP(ip)
		if err != nil {
			t.Fatalf("Unexpected error for %s: %v", ip, err)
		}

		if existingIP, exists := seen[mac]; exists {
			t.Errorf("MAC collision! IP %s and %s both produced MAC %s", ip, existingIP, mac)
		}
		seen[mac] = ip
	}
}

// TestCalculateMACFromIP_Deterministic verifies that the same IP always produces the same MAC
func TestCalculateMACFromIP_Deterministic(t *testing.T) {
	testIP := "10.20.30.40/24"

	// Calculate MAC multiple times
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		mac, err := calculateMACFromIP(testIP)
		if err != nil {
			t.Fatalf("Unexpected error on iteration %d: %v", i, err)
		}
		results[i] = mac
	}

	// Verify all results are identical
	expected := results[0]
	for i, mac := range results {
		if mac != expected {
			t.Errorf("Iteration %d: expected %s, got %s (not deterministic!)", i, expected, mac)
		}
	}
}

// TestCalculateMACFromIP_CIDRIndependence verifies that CIDR suffix doesn't affect MAC
func TestCalculateMACFromIP_CIDRIndependence(t *testing.T) {
	testCases := []struct {
		ips []string // All should produce the same MAC
		mac string
	}{
		{
			ips: []string{"10.0.0.1", "10.0.0.1/24", "10.0.0.1/32", "10.0.0.1/16"},
			mac: "be:ef:0a:00:00:01",
		},
		{
			ips: []string{"192.168.1.50", "192.168.1.50/24", "192.168.1.50/31", "192.168.1.50/8"},
			mac: "be:ef:c0:a8:01:32",
		},
	}

	for _, tc := range testCases {
		for _, ip := range tc.ips {
			mac, err := calculateMACFromIP(ip)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", ip, err)
				continue
			}
			if mac != tc.mac {
				t.Errorf("For IP %s, expected MAC %s, got %s", ip, tc.mac, mac)
			}
		}
	}
}

// Normalization Tests

func TestNormalize(t *testing.T) {
	tests := []struct {
		name           string
		input          VMConfig
		expectedName   string
		expectedFQDN   string
		expectedBridge string
	}{
		{
			name: "uppercase name to lowercase",
			input: VMConfig{
				Name:      "MyVM",
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "br0"},
				},
			},
			expectedName:   "myvm",
			expectedBridge: "br0",
		},
		{
			name: "mixed case name with spaces",
			input: VMConfig{
				Name:      "  Test-VM  ",
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "br0"},
				},
			},
			expectedName:   "test-vm",
			expectedBridge: "br0",
		},
		{
			name: "uppercase FQDN to lowercase",
			input: VMConfig{
				Name:      "test-vm",
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "br0"},
				},
				CloudInit: &CloudInitConfig{
					FQDN: "Test-VM.Example.COM",
				},
			},
			expectedName: "test-vm",
			expectedFQDN: "test-vm.example.com",
		},
		{
			name: "preserve bridge case (must match hypervisor)",
			input: VMConfig{
				Name:      "TEST",
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "BR0"},
				},
			},
			expectedName:   "test",
			expectedBridge: "BR0", // Bridge should NOT be normalized
		},
		{
			name: "no cloud-init config",
			input: VMConfig{
				Name:      "VM-Name",
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "br0"},
				},
			},
			expectedName:   "vm-name",
			expectedBridge: "br0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.input
			config.Normalize()

			if config.Name != tt.expectedName {
				t.Errorf("Name: expected %q, got %q", tt.expectedName, config.Name)
			}

			if tt.expectedFQDN != "" && config.CloudInit != nil {
				if config.CloudInit.FQDN != tt.expectedFQDN {
					t.Errorf("FQDN: expected %q, got %q", tt.expectedFQDN, config.CloudInit.FQDN)
				}
			}

			if tt.expectedBridge != "" && len(config.Network) > 0 {
				if config.Network[0].Bridge != tt.expectedBridge {
					t.Errorf("Bridge: expected %q, got %q", tt.expectedBridge, config.Network[0].Bridge)
				}
			}
		})
	}
}

// VM Name Validation Tests

func TestVMNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		vmName    string
		expectErr bool
	}{
		{
			name:      "valid simple name",
			vmName:    "myvm",
			expectErr: false,
		},
		{
			name:      "valid name with hyphen",
			vmName:    "my-vm",
			expectErr: false,
		},
		{
			name:      "valid name with underscore",
			vmName:    "my_vm",
			expectErr: false,
		},
		{
			name:      "valid name with numbers",
			vmName:    "vm01",
			expectErr: false,
		},
		{
			name:      "valid complex name",
			vmName:    "prod-web01",
			expectErr: false,
		},
		{
			name:      "valid single character",
			vmName:    "v",
			expectErr: false,
		},
		{
			name:      "invalid - starts with hyphen",
			vmName:    "-myvm",
			expectErr: true,
		},
		{
			name:      "invalid - ends with hyphen",
			vmName:    "myvm-",
			expectErr: true,
		},
		{
			name:      "invalid - starts with underscore",
			vmName:    "_myvm",
			expectErr: true,
		},
		{
			name:      "invalid - ends with underscore",
			vmName:    "myvm_",
			expectErr: true,
		},
		{
			name:      "invalid - contains uppercase (should be normalized first)",
			vmName:    "MyVM",
			expectErr: true,
		},
		{
			name:      "invalid - contains dot",
			vmName:    "my.vm",
			expectErr: true,
		},
		{
			name:      "invalid - contains space",
			vmName:    "my vm",
			expectErr: true,
		},
		{
			name:      "invalid - contains slash",
			vmName:    "my/vm",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := VMConfig{
				Name:      tt.vmName,
				VCPUs:     1,
				MemoryGiB: 1,
				BootDisk:  BootDiskConfig{SizeGB: 10, Empty: true},
				Network: []NetworkInterface{
					{IP: "10.0.0.1/24", Gateway: "10.0.0.1", Bridge: "br0"},
				},
			}

			err := config.Validate()
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected validation error for name %q, got nil", tt.vmName)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for name %q, got: %v", tt.vmName, err)
				}
			}
		})
	}
}

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name           string
		image          string
		imagePool      string
		expectPool     string
		expectVolume   string
		expectFilePath bool
		expectErr      bool
	}{
		{
			name:           "volume name only - uses default pool",
			image:          "fedora-43",
			imagePool:      "",
			expectPool:     "foundry-images",
			expectVolume:   "fedora-43",
			expectFilePath: false,
			expectErr:      false,
		},
		{
			name:           "volume name only - uses specified pool",
			image:          "ubuntu-24.04",
			imagePool:      "custom-images",
			expectPool:     "custom-images",
			expectVolume:   "ubuntu-24.04",
			expectFilePath: false,
			expectErr:      false,
		},
		{
			name:           "pool:volume format",
			image:          "custom-pool:debian-12",
			imagePool:      "foundry-images",
			expectPool:     "custom-pool",
			expectVolume:   "debian-12",
			expectFilePath: false,
			expectErr:      false,
		},
		{
			name:           "pool:volume format with spaces",
			image:          " my-pool : my-volume ",
			imagePool:      "",
			expectPool:     "my-pool",
			expectVolume:   "my-volume",
			expectFilePath: false,
			expectErr:      false,
		},
		{
			name:           "absolute file path",
			image:          "/var/lib/libvirt/images/fedora.qcow2",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: true,
			expectErr:      false,
		},
		{
			name:           "relative file path",
			image:          "./images/ubuntu.qcow2",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: true,
			expectErr:      false,
		},
		{
			name:           "relative path with parent",
			image:          "../base/fedora.qcow2",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: true,
			expectErr:      false,
		},
		{
			name:           "empty image",
			image:          "",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: false,
			expectErr:      false,
		},
		{
			name:           "invalid pool:volume - empty pool",
			image:          ":volume",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: false,
			expectErr:      true,
		},
		{
			name:           "invalid pool:volume - empty volume",
			image:          "pool:",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: false,
			expectErr:      true,
		},
		{
			name:           "invalid pool:volume - spaces only",
			image:          "  :  ",
			imagePool:      "",
			expectPool:     "",
			expectVolume:   "",
			expectFilePath: false,
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bootDisk := BootDiskConfig{
				Image:     tt.image,
				ImagePool: tt.imagePool,
			}

			pool, volume, isFilePath, err := bootDisk.ParseImageReference()

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if pool != tt.expectPool {
				t.Errorf("Expected pool %q, got %q", tt.expectPool, pool)
			}
			if volume != tt.expectVolume {
				t.Errorf("Expected volume %q, got %q", tt.expectVolume, volume)
			}
			if isFilePath != tt.expectFilePath {
				t.Errorf("Expected isFilePath %v, got %v", tt.expectFilePath, isFilePath)
			}
		})
	}
}

func TestGetStoragePool(t *testing.T) {
	tests := []struct {
		name        string
		storagePool string
		expected    string
	}{
		{
			name:        "default when empty",
			storagePool: "",
			expected:    "foundry-vms",
		},
		{
			name:        "custom pool",
			storagePool: "my-custom-pool",
			expected:    "my-custom-pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &VMConfig{StoragePool: tt.storagePool}
			result := config.GetStoragePool()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVolumeNameHelpers(t *testing.T) {
	config := &VMConfig{Name: "test-vm"}

	t.Run("GetBootVolumeName", func(t *testing.T) {
		expected := "test-vm_boot.qcow2"
		result := config.GetBootVolumeName()
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("GetDataVolumeName", func(t *testing.T) {
		expected := "test-vm_data-vdb.qcow2"
		result := config.GetDataVolumeName("vdb")
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("GetCloudInitVolumeName", func(t *testing.T) {
		expected := "test-vm_cloudinit.iso"
		result := config.GetCloudInitVolumeName()
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

func TestNormalize_SetsDefaults(t *testing.T) {
	config := &VMConfig{
		Name:      "Test-VM",
		BootDisk:  BootDiskConfig{},
		CloudInit: &CloudInitConfig{FQDN: "Test.Example.Com"},
	}

	config.Normalize()

	// Check name normalization
	if config.Name != "test-vm" {
		t.Errorf("Expected name 'test-vm', got %q", config.Name)
	}

	// Check FQDN normalization
	if config.CloudInit.FQDN != "test.example.com" {
		t.Errorf("Expected FQDN 'test.example.com', got %q", config.CloudInit.FQDN)
	}

	// Check storage pool default
	if config.StoragePool != "foundry-vms" {
		t.Errorf("Expected StoragePool 'foundry-vms', got %q", config.StoragePool)
	}

	// Check image pool default
	if config.BootDisk.ImagePool != "foundry-images" {
		t.Errorf("Expected ImagePool 'foundry-images', got %q", config.BootDisk.ImagePool)
	}
}

func TestNormalize_PreservesExplicitPools(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		StoragePool: "custom-vms",
		BootDisk: BootDiskConfig{
			ImagePool: "custom-images",
		},
	}

	config.Normalize()

	// Check custom pools are preserved
	if config.StoragePool != "custom-vms" {
		t.Errorf("Expected StoragePool 'custom-vms', got %q", config.StoragePool)
	}
	if config.BootDisk.ImagePool != "custom-images" {
		t.Errorf("Expected ImagePool 'custom-images', got %q", config.BootDisk.ImagePool)
	}
}
