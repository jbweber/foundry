package cloudinit

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jbweber/plow/internal/config"
)

// Test SSH keys (valid keys generated for testing)
const (
	testSSHKeyEd25519 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIbJKZscbOLzBsgY5y2QupKW4A2kSDjMBQGPb1dChr+S test@example.com"
	testSSHKeyRSA     = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCq7mGKPGMc36QAe7g1dJ8oGeDD1VnfBwdC3YAlp8zX3cQm8PEaaBUsKgVPigiFVWMwKTBpP2YWAjQaqyBIgFM7sneE8Ke3ouMS9GaOoFHMcorvX1N6oJtldL58D1vfGpHcBfwZiSFHxHZOZwG0Q0hCBJcoAiVtBUaubspLiXY/QgUZnw1JgbAsVuFdHxMsqSwi8NC6smVhg00T28TDubfgMZM02Uvd/qNZF6PzKxUhcCIY4zCHtsiMeN7njssKmjnuBLBlD51D19Rw6CbHsKOEskdpIHU+8o5debIwHk7c6Q0iOGTs/2lg/Rjzs+Us59NOTRB+jECEAbO0r19l//pr test-rsa@example.com"
)

func TestGenerateUserData(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.VMConfig
		expectErr    bool
		checkContent func(t *testing.T, content string)
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectErr: true,
		},
		{
			name: "minimal config - no cloud-init",
			cfg: &config.VMConfig{
				Name: "test-vm",
			},
			checkContent: func(t *testing.T, content string) {
				// Must start with #cloud-config
				if !strings.HasPrefix(content, "#cloud-config\n") {
					t.Error("user-data must start with '#cloud-config'")
				}

				// Parse YAML
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				// Verify fields
				if userData.Hostname != "test-vm" {
					t.Errorf("Expected hostname 'test-vm', got %q", userData.Hostname)
				}
				if userData.FQDN != "test-vm" {
					t.Errorf("Expected fqdn 'test-vm', got %q", userData.FQDN)
				}
				if userData.SSHPasswordAuth != false {
					t.Errorf("Expected ssh_pwauth false, got %v", userData.SSHPasswordAuth)
				}
				if userData.Output == nil || userData.Output.All != "| tee -a /var/log/cloud-init-output.log" {
					t.Error("Expected output logging to be configured")
				}
			},
		},
		{
			name: "with FQDN - hostname extraction",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					FQDN: "web01.prod.example.com",
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				// Hostname should be extracted from FQDN (everything before first dot)
				if userData.Hostname != "web01" {
					t.Errorf("Expected hostname 'web01', got %q", userData.Hostname)
				}
				if userData.FQDN != "web01.prod.example.com" {
					t.Errorf("Expected fqdn 'web01.prod.example.com', got %q", userData.FQDN)
				}
			},
		},
		{
			name: "with SSH keys",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					SSHKeys: []string{testSSHKeyEd25519, testSSHKeyRSA},
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				if len(userData.SSHAuthorizedKeys) != 2 {
					t.Errorf("Expected 2 SSH keys, got %d", len(userData.SSHAuthorizedKeys))
				}
				if userData.SSHAuthorizedKeys[0] != testSSHKeyEd25519 {
					t.Error("First SSH key doesn't match")
				}
				if userData.SSHAuthorizedKeys[1] != testSSHKeyRSA {
					t.Error("Second SSH key doesn't match")
				}
			},
		},
		{
			name: "with root password hash",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					RootPasswordHash: "$6$rounds=4096$salt$hashedpassword",
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				if userData.Chpasswd == nil {
					t.Fatal("Expected chpasswd to be set")
				}
				if userData.Chpasswd.Expire != false {
					t.Error("Expected chpasswd.expire to be false")
				}
				expectedList := "root:$6$rounds=4096$salt$hashedpassword"
				if userData.Chpasswd.List != expectedList {
					t.Errorf("Expected chpasswd.list %q, got %q", expectedList, userData.Chpasswd.List)
				}
			},
		},
		{
			name: "ssh_pwauth enabled",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					SSHPwAuth: boolPtr(true),
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				if userData.SSHPasswordAuth != true {
					t.Errorf("Expected ssh_pwauth true, got %v", userData.SSHPasswordAuth)
				}
			},
		},
		{
			name: "ssh_pwauth explicitly disabled",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					SSHPwAuth: boolPtr(false),
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				if userData.SSHPasswordAuth != false {
					t.Errorf("Expected ssh_pwauth false, got %v", userData.SSHPasswordAuth)
				}
			},
		},
		{
			name: "complete config",
			cfg: &config.VMConfig{
				Name: "test-vm",
				CloudInit: &config.CloudInitConfig{
					FQDN:             "test-vm.example.com",
					SSHKeys:          []string{testSSHKeyEd25519},
					RootPasswordHash: "$6$rounds=4096$salt$hashedpassword",
					SSHPwAuth:        boolPtr(false),
				},
			},
			checkContent: func(t *testing.T, content string) {
				var userData UserData
				if err := yaml.Unmarshal([]byte(strings.TrimPrefix(content, "#cloud-config\n")), &userData); err != nil {
					t.Fatalf("Failed to parse user-data YAML: %v", err)
				}

				if userData.Hostname != "test-vm" {
					t.Errorf("Expected hostname 'test-vm', got %q", userData.Hostname)
				}
				if len(userData.SSHAuthorizedKeys) != 1 {
					t.Errorf("Expected 1 SSH key, got %d", len(userData.SSHAuthorizedKeys))
				}
				if userData.Chpasswd == nil {
					t.Error("Expected chpasswd to be set")
				}
				if userData.SSHPasswordAuth != false {
					t.Error("Expected ssh_pwauth false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GenerateUserData(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.checkContent != nil {
				tt.checkContent(t, content)
			}
		})
	}
}

func TestGenerateMetaData(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.VMConfig
		expectErr    bool
		checkContent func(t *testing.T, content string, vmName string)
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectErr: true,
		},
		{
			name: "valid config",
			cfg: &config.VMConfig{
				Name: "test-vm",
			},
			checkContent: func(t *testing.T, content string, vmName string) {
				var metaData MetaData
				if err := yaml.Unmarshal([]byte(content), &metaData); err != nil {
					t.Fatalf("Failed to parse meta-data YAML: %v", err)
				}

				// Instance ID should be the VM name
				if metaData.InstanceID != vmName {
					t.Errorf("Expected instance-id %q, got %q", vmName, metaData.InstanceID)
				}

				if metaData.LocalHostname != vmName {
					t.Errorf("Expected local-hostname %q, got %q", vmName, metaData.LocalHostname)
				}
			},
		},
		{
			name: "different VM name",
			cfg: &config.VMConfig{
				Name: "prod-web01",
			},
			checkContent: func(t *testing.T, content string, vmName string) {
				var metaData MetaData
				if err := yaml.Unmarshal([]byte(content), &metaData); err != nil {
					t.Fatalf("Failed to parse meta-data YAML: %v", err)
				}

				if metaData.InstanceID != "prod-web01" {
					t.Errorf("Expected instance-id 'prod-web01', got %q", metaData.InstanceID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GenerateMetaData(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.checkContent != nil {
				tt.checkContent(t, content, tt.cfg.Name)
			}
		})
	}
}

func TestGenerateNetworkConfig(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.VMConfig
		expectErr    bool
		checkContent func(t *testing.T, content string)
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectErr: true,
		},
		{
			name: "empty network interfaces",
			cfg: &config.VMConfig{
				Name:    "test-vm",
				Network: []config.NetworkInterface{},
			},
			expectErr: true,
		},
		{
			name: "single interface with default route",
			cfg: &config.VMConfig{
				Name: "test-vm",
				Network: []config.NetworkInterface{
					{
						IP:           "10.20.30.40/24",
						Gateway:      "10.20.30.1",
						DNSServers:   []string{"8.8.8.8", "1.1.1.1"},
						MACAddress:   "be:ef:0a:14:1e:28",
						DefaultRoute: true,
					},
				},
			},
			checkContent: func(t *testing.T, content string) {
				var netConfig NetworkConfig
				if err := yaml.Unmarshal([]byte(content), &netConfig); err != nil {
					t.Fatalf("Failed to parse network-config YAML: %v", err)
				}

				if netConfig.Version != 2 {
					t.Errorf("Expected version 2, got %d", netConfig.Version)
				}

				eth0, ok := netConfig.Ethernets["eth0"]
				if !ok {
					t.Fatal("Expected eth0 interface")
				}

				if eth0.Match.MACAddress != "be:ef:0a:14:1e:28" {
					t.Errorf("Expected MAC 'be:ef:0a:14:1e:28', got %q", eth0.Match.MACAddress)
				}

				if len(eth0.Addresses) != 1 || eth0.Addresses[0] != "10.20.30.40/24" {
					t.Errorf("Expected address '10.20.30.40/24', got %v", eth0.Addresses)
				}

				if len(eth0.Routes) != 1 {
					t.Fatalf("Expected 1 route, got %d", len(eth0.Routes))
				}
				if eth0.Routes[0].To != "0.0.0.0/0" {
					t.Errorf("Expected route to '0.0.0.0/0', got %q", eth0.Routes[0].To)
				}
				if eth0.Routes[0].Via != "10.20.30.1" {
					t.Errorf("Expected route via '10.20.30.1', got %q", eth0.Routes[0].Via)
				}

				if eth0.Nameservers == nil || len(eth0.Nameservers.Addresses) != 2 {
					t.Error("Expected 2 DNS servers")
				}
			},
		},
		{
			name: "single interface without default route",
			cfg: &config.VMConfig{
				Name: "test-vm",
				Network: []config.NetworkInterface{
					{
						IP:           "10.20.30.40/24",
						Gateway:      "10.20.30.1",
						MACAddress:   "be:ef:0a:14:1e:28",
						DefaultRoute: false,
					},
				},
			},
			checkContent: func(t *testing.T, content string) {
				var netConfig NetworkConfig
				if err := yaml.Unmarshal([]byte(content), &netConfig); err != nil {
					t.Fatalf("Failed to parse network-config YAML: %v", err)
				}

				eth0 := netConfig.Ethernets["eth0"]
				if len(eth0.Routes) != 0 {
					t.Errorf("Expected no routes when default_route is false, got %d", len(eth0.Routes))
				}
			},
		},
		{
			name: "interface without DNS servers",
			cfg: &config.VMConfig{
				Name: "test-vm",
				Network: []config.NetworkInterface{
					{
						IP:           "10.20.30.40/24",
						Gateway:      "10.20.30.1",
						MACAddress:   "be:ef:0a:14:1e:28",
						DefaultRoute: true,
					},
				},
			},
			checkContent: func(t *testing.T, content string) {
				var netConfig NetworkConfig
				if err := yaml.Unmarshal([]byte(content), &netConfig); err != nil {
					t.Fatalf("Failed to parse network-config YAML: %v", err)
				}

				eth0 := netConfig.Ethernets["eth0"]
				if eth0.Nameservers != nil {
					t.Error("Expected no nameservers when DNS servers not configured")
				}
			},
		},
		{
			name: "multiple interfaces",
			cfg: &config.VMConfig{
				Name: "test-vm",
				Network: []config.NetworkInterface{
					{
						IP:           "10.20.30.40/24",
						Gateway:      "10.20.30.1",
						DNSServers:   []string{"8.8.8.8"},
						MACAddress:   "be:ef:0a:14:1e:28",
						DefaultRoute: true,
					},
					{
						IP:           "192.168.1.50/24",
						Gateway:      "192.168.1.1",
						DNSServers:   []string{"192.168.1.1"},
						MACAddress:   "be:ef:c0:a8:01:32",
						DefaultRoute: false,
					},
				},
			},
			checkContent: func(t *testing.T, content string) {
				var netConfig NetworkConfig
				if err := yaml.Unmarshal([]byte(content), &netConfig); err != nil {
					t.Fatalf("Failed to parse network-config YAML: %v", err)
				}

				if len(netConfig.Ethernets) != 2 {
					t.Errorf("Expected 2 interfaces, got %d", len(netConfig.Ethernets))
				}

				// Check eth0
				eth0, ok := netConfig.Ethernets["eth0"]
				if !ok {
					t.Fatal("Expected eth0 interface")
				}
				if len(eth0.Routes) != 1 {
					t.Error("Expected eth0 to have default route")
				}

				// Check eth1
				eth1, ok := netConfig.Ethernets["eth1"]
				if !ok {
					t.Fatal("Expected eth1 interface")
				}
				if eth1.Match.MACAddress != "be:ef:c0:a8:01:32" {
					t.Errorf("Expected eth1 MAC 'be:ef:c0:a8:01:32', got %q", eth1.Match.MACAddress)
				}
				if len(eth1.Routes) != 0 {
					t.Error("Expected eth1 to have no default route")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GenerateNetworkConfig(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.checkContent != nil {
				tt.checkContent(t, content)
			}
		})
	}
}

// TestGenerateAll tests generating all three cloud-init files from the same config
func TestGenerateAll(t *testing.T) {
	cfg := &config.VMConfig{
		Name:      "integration-test",
		VCPUs:     4,
		MemoryGiB: 8,
		Network: []config.NetworkInterface{
			{
				IP:           "10.55.22.22/24",
				Gateway:      "10.55.22.1",
				DNSServers:   []string{"8.8.8.8", "1.1.1.1"},
				MACAddress:   "be:ef:0a:37:16:16",
				DefaultRoute: true,
			},
		},
		CloudInit: &config.CloudInitConfig{
			FQDN:             "integration-test.example.com",
			SSHKeys:          []string{testSSHKeyEd25519},
			RootPasswordHash: "$6$rounds=4096$salt$hashedpassword",
			SSHPwAuth:        boolPtr(false),
		},
	}

	// Generate all three files
	userData, err := GenerateUserData(cfg)
	if err != nil {
		t.Fatalf("GenerateUserData failed: %v", err)
	}

	metaData, err := GenerateMetaData(cfg)
	if err != nil {
		t.Fatalf("GenerateMetaData failed: %v", err)
	}

	networkConfig, err := GenerateNetworkConfig(cfg)
	if err != nil {
		t.Fatalf("GenerateNetworkConfig failed: %v", err)
	}

	// Verify all are non-empty
	if len(userData) == 0 {
		t.Error("user-data is empty")
	}
	if len(metaData) == 0 {
		t.Error("meta-data is empty")
	}
	if len(networkConfig) == 0 {
		t.Error("network-config is empty")
	}

	// Verify user-data starts with header
	if !strings.HasPrefix(userData, "#cloud-config\n") {
		t.Error("user-data missing #cloud-config header")
	}

	// Parse and verify consistency
	var parsedUserData UserData
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(userData, "#cloud-config\n")), &parsedUserData); err != nil {
		t.Fatalf("Failed to parse user-data: %v", err)
	}

	var parsedMetaData MetaData
	if err := yaml.Unmarshal([]byte(metaData), &parsedMetaData); err != nil {
		t.Fatalf("Failed to parse meta-data: %v", err)
	}

	var parsedNetworkConfig NetworkConfig
	if err := yaml.Unmarshal([]byte(networkConfig), &parsedNetworkConfig); err != nil {
		t.Fatalf("Failed to parse network-config: %v", err)
	}

	// Verify hostname consistency
	if parsedUserData.Hostname != "integration-test" {
		t.Errorf("user-data hostname mismatch: got %q", parsedUserData.Hostname)
	}
	if parsedMetaData.LocalHostname != "integration-test" {
		t.Errorf("meta-data local-hostname mismatch: got %q", parsedMetaData.LocalHostname)
	}

	// Verify network config has correct MAC
	eth0 := parsedNetworkConfig.Ethernets["eth0"]
	if eth0.Match.MACAddress != "be:ef:0a:37:16:16" {
		t.Errorf("network-config MAC mismatch: got %q", eth0.Match.MACAddress)
	}
}

// boolPtr is a helper to create a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
