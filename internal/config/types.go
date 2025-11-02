package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// VMConfig represents the complete VM configuration.
type VMConfig struct {
	Name        string             `yaml:"name"`
	VCPUs       int                `yaml:"vcpus"`
	MemoryGiB   int                `yaml:"memory_gib"`
	BootDisk    BootDiskConfig     `yaml:"boot_disk"`
	DataDisks   []DataDiskConfig   `yaml:"data_disks,omitempty"`
	Network     []NetworkInterface `yaml:"network_interfaces"`
	CloudInit   *CloudInitConfig   `yaml:"cloud_init,omitempty"`
	StoragePool string             `yaml:"storage_pool,omitempty"` // Libvirt storage pool for VM volumes (default: "foundry-vms")
}

// BootDiskConfig defines the boot disk configuration.
type BootDiskConfig struct {
	SizeGB    int    `yaml:"size_gb"`
	Image     string `yaml:"image,omitempty"`      // Image reference (volume name, pool:volume, or file path)
	ImagePool string `yaml:"image_pool,omitempty"` // Pool for base images (default: "foundry-images")
	Empty     bool   `yaml:"empty,omitempty"`      // Create empty disk instead
}

// DataDiskConfig defines an additional data disk.
type DataDiskConfig struct {
	Device string `yaml:"device"` // vdb, vdc, etc.
	SizeGB int    `yaml:"size_gb"`
}

// NetworkInterface defines a network interface configuration.
type NetworkInterface struct {
	IP           string   `yaml:"ip"` // IP with CIDR, e.g., "10.20.30.40/24"
	Gateway      string   `yaml:"gateway"`
	DNSServers   []string `yaml:"dns_servers"`
	Bridge       string   `yaml:"bridge"`
	DefaultRoute bool     `yaml:"default_route,omitempty"` // Set default route via this interface

	// Derived fields (not in YAML, calculated from IP)
	MACAddress string `yaml:"-"` // Will be calculated: be:ef:0a:14:1e:28
}

// CloudInitConfig contains cloud-init configuration.
// Follows cloud-init spec: https://cloudinit.readthedocs.io/
// Note: Hostname is derived from FQDN (everything before the first dot).
type CloudInitConfig struct {
	FQDN             string   `yaml:"fqdn,omitempty"`
	SSHKeys          []string `yaml:"ssh_keys,omitempty"`
	RootPasswordHash string   `yaml:"root_password_hash,omitempty"`
	SSHPwAuth        *bool    `yaml:"ssh_pwauth,omitempty"` // Pointer to distinguish unset vs false
}

// Validate checks the configuration for errors.
// Does not validate hypervisor resources (images, bridges, etc.) - only config structure.
func (c *VMConfig) Validate() error {
	// Check required fields
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate name format (after normalization to lowercase)
	// Must start and end with alphanumeric, can contain alphanumeric, hyphens, underscores
	// Pattern matches libvirt domain name requirements
	namePattern := `^[a-z0-9][a-z0-9_-]*[a-z0-9]$`
	if len(c.Name) == 1 {
		// Single character names just need to be alphanumeric
		namePattern = `^[a-z0-9]$`
	}
	matched, err := regexp.MatchString(namePattern, c.Name)
	if err != nil {
		return fmt.Errorf("name validation error: %w", err)
	}
	if !matched {
		return fmt.Errorf("name must start and end with alphanumeric characters and contain only alphanumeric, hyphens, or underscores, got %q", c.Name)
	}

	if c.VCPUs <= 0 {
		return fmt.Errorf("vcpus must be > 0, got %d", c.VCPUs)
	}
	if c.MemoryGiB <= 0 {
		return fmt.Errorf("memory_gib must be > 0, got %d", c.MemoryGiB)
	}

	// Validate boot disk
	if err := c.BootDisk.Validate(); err != nil {
		return fmt.Errorf("boot_disk: %w", err)
	}

	// Validate data disks
	devicesSeen := make(map[string]bool)
	for i, disk := range c.DataDisks {
		if err := disk.Validate(); err != nil {
			return fmt.Errorf("data_disks[%d]: %w", i, err)
		}
		// Check for duplicate device names
		if devicesSeen[disk.Device] {
			return fmt.Errorf("data_disks[%d]: duplicate device name %q", i, disk.Device)
		}
		devicesSeen[disk.Device] = true
	}

	// Validate network interfaces
	if len(c.Network) == 0 {
		return fmt.Errorf("at least one network_interfaces entry is required")
	}
	ipsSeen := make(map[string]bool)
	for i, iface := range c.Network {
		if err := iface.Validate(); err != nil {
			return fmt.Errorf("network_interfaces[%d]: %w", i, err)
		}
		// Check for duplicate IPs
		if ipsSeen[iface.IP] {
			return fmt.Errorf("network_interfaces[%d]: duplicate IP %q", i, iface.IP)
		}
		ipsSeen[iface.IP] = true
	}

	// Validate cloud-init config if present
	if c.CloudInit != nil {
		if err := c.CloudInit.Validate(); err != nil {
			return fmt.Errorf("cloud_init: %w", err)
		}
	}

	return nil
}

// Validate checks boot disk configuration.
func (b *BootDiskConfig) Validate() error {
	if b.SizeGB <= 0 {
		return fmt.Errorf("size_gb must be > 0, got %d", b.SizeGB)
	}
	if b.Image == "" && !b.Empty {
		return fmt.Errorf("must specify either 'image' or 'empty: true'")
	}
	if b.Image != "" && b.Empty {
		return fmt.Errorf("cannot specify both 'image' and 'empty: true'")
	}
	return nil
}

// Validate checks data disk configuration.
func (d *DataDiskConfig) Validate() error {
	if d.Device == "" {
		return fmt.Errorf("device is required")
	}
	if d.SizeGB <= 0 {
		return fmt.Errorf("size_gb must be > 0, got %d", d.SizeGB)
	}
	return nil
}

// Validate checks network interface configuration.
func (n *NetworkInterface) Validate() error {
	if n.IP == "" {
		return fmt.Errorf("ip is required")
	}
	if n.Gateway == "" {
		return fmt.Errorf("gateway is required")
	}
	if n.Bridge == "" {
		return fmt.Errorf("bridge is required")
	}

	// Validate IP/CIDR format
	ip, ipnet, err := net.ParseCIDR(n.IP)
	if err != nil {
		return fmt.Errorf("invalid ip/cidr format %q: %w", n.IP, err)
	}
	if ip == nil || ipnet == nil {
		return fmt.Errorf("invalid ip/cidr format %q", n.IP)
	}

	// Validate gateway is an IP
	if net.ParseIP(n.Gateway) == nil {
		return fmt.Errorf("invalid gateway IP address %q", n.Gateway)
	}

	// Validate DNS servers
	for i, dns := range n.DNSServers {
		if net.ParseIP(dns) == nil {
			return fmt.Errorf("dns_servers[%d] is not a valid IP address: %q", i, dns)
		}
	}

	return nil
}

// Validate checks cloud-init configuration.
func (c *CloudInitConfig) Validate() error {
	// Validate FQDN format if provided
	if c.FQDN != "" {
		// FQDN must be a valid hostname with at least one dot
		// RFC 952/1123: alphanumeric and hyphens, labels separated by dots
		// Each label: 1-63 chars, start/end with alphanumeric
		fqdnPattern := `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`
		matched, err := regexp.MatchString(fqdnPattern, c.FQDN)
		if err != nil {
			return fmt.Errorf("fqdn validation error: %w", err)
		}
		if !matched {
			return fmt.Errorf("fqdn must be a valid hostname with domain (e.g., host.example.com), got %q", c.FQDN)
		}
	}

	// Validate SSH keys using golang.org/x/crypto/ssh parser
	for i, key := range c.SSHKeys {
		// ParseAuthorizedKey validates the key format and can parse all standard SSH key types
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return fmt.Errorf("ssh_keys[%d] is not a valid SSH public key: %w", i, err)
		}
	}

	// Validate password hash format if provided
	if c.RootPasswordHash != "" {
		if len(c.RootPasswordHash) < 10 || c.RootPasswordHash[0] != '$' {
			return fmt.Errorf("root_password_hash must be a valid crypt hash (should start with $)")
		}
	}

	return nil
}

// GetBootDiskPath returns the libvirt storage volume path for the boot disk.
// Format: <vm-name>/boot.qcow2
// These helpers encapsulate the naming pattern in one place, making it easy to:
// - Ensure consistency across storage creation, domain XML, and cleanup
// - Change the naming pattern or base path in the future (Phase 2: configurable storage)
func (c *VMConfig) GetBootDiskPath() string {
	return fmt.Sprintf("%s/boot.qcow2", c.Name)
}

// GetDataDiskPath returns the libvirt storage volume path for a data disk.
// Format: <vm-name>/data-<device>.qcow2 (e.g., "my-vm/data-vdb.qcow2")
func (c *VMConfig) GetDataDiskPath(device string) string {
	return fmt.Sprintf("%s/data-%s.qcow2", c.Name, device)
}

// GetCloudInitISOPath returns the libvirt storage volume path for the cloud-init ISO.
// Format: <vm-name>/cloudinit.iso
func (c *VMConfig) GetCloudInitISOPath() string {
	return fmt.Sprintf("%s/cloudinit.iso", c.Name)
}

// GetVMDirectory returns the VM's storage directory path.
// Format: <vm-name>
// Used for creating the VM directory and cleanup operations.
func (c *VMConfig) GetVMDirectory() string {
	return c.Name
}

// GetBootVolumeName returns the volume name for the boot disk.
// Format: <vm-name>_boot.qcow2 (includes extension to indicate format)
func (c *VMConfig) GetBootVolumeName() string {
	return fmt.Sprintf("%s_boot.qcow2", c.Name)
}

// GetDataVolumeName returns the volume name for a data disk.
// Format: <vm-name>_data-<device>.qcow2 (e.g., "my-vm_data-vdb.qcow2")
func (c *VMConfig) GetDataVolumeName(device string) string {
	return fmt.Sprintf("%s_data-%s.qcow2", c.Name, device)
}

// GetCloudInitVolumeName returns the volume name for the cloud-init ISO.
// Format: <vm-name>_cloudinit.iso (includes extension to indicate format)
func (c *VMConfig) GetCloudInitVolumeName() string {
	return fmt.Sprintf("%s_cloudinit.iso", c.Name)
}

// Normalize sanitizes user input to consistent formats.
// This is called automatically by LoadFromFile before validation.
func (c *VMConfig) Normalize() {
	// Normalize VM name to lowercase
	c.Name = strings.ToLower(strings.TrimSpace(c.Name))

	// Normalize cloud-init FQDN to lowercase (hostname will be derived from this)
	if c.CloudInit != nil {
		c.CloudInit.FQDN = strings.ToLower(strings.TrimSpace(c.CloudInit.FQDN))
	}

	// Note: Bridge names are NOT normalized - they must match hypervisor config exactly

	// Set default storage pool if not specified
	if c.StoragePool == "" {
		c.StoragePool = "foundry-vms"
	}

	// Set default image pool if not specified
	if c.BootDisk.ImagePool == "" {
		c.BootDisk.ImagePool = "foundry-images"
	}
}

// GetStoragePool returns the storage pool name for VM volumes, using default if not set.
func (c *VMConfig) GetStoragePool() string {
	if c.StoragePool == "" {
		return "foundry-vms"
	}
	return c.StoragePool
}

// ParseImageReference parses an image reference and returns the pool and volume names.
// Supports three formats:
//   - Volume name only: "fedora-43" -> uses ImagePool (or "foundry-images" default)
//   - Pool:volume format: "foundry-images:fedora-43" -> explicit pool and volume
//   - File path: "/var/lib/libvirt/images/fedora.qcow2" -> returns empty strings (backward compat)
//
// Returns: (poolName, volumeName, isFilePath, error)
func (b *BootDiskConfig) ParseImageReference() (string, string, bool, error) {
	if b.Image == "" {
		return "", "", false, nil
	}

	// Check if it's a file path (contains / or starts with .)
	if strings.Contains(b.Image, "/") || strings.HasPrefix(b.Image, ".") {
		return "", "", true, nil
	}

	// Check for pool:volume format
	if strings.Contains(b.Image, ":") {
		parts := strings.SplitN(b.Image, ":", 2)
		if len(parts) != 2 {
			return "", "", false, fmt.Errorf("invalid pool:volume format: %q", b.Image)
		}
		poolName := strings.TrimSpace(parts[0])
		volumeName := strings.TrimSpace(parts[1])
		if poolName == "" || volumeName == "" {
			return "", "", false, fmt.Errorf("invalid pool:volume format: pool and volume cannot be empty")
		}
		return poolName, volumeName, false, nil
	}

	// Just a volume name - use ImagePool (or default)
	imagePool := b.ImagePool
	if imagePool == "" {
		imagePool = "foundry-images"
	}
	return imagePool, b.Image, false, nil
}

// LoadFromFile loads a VM configuration from a YAML file.
func LoadFromFile(path string) (*VMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config VMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Normalize user input before validation
	config.Normalize()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Calculate MAC addresses for all network interfaces
	if err := config.CalculateMACs(); err != nil {
		return nil, fmt.Errorf("failed to calculate MAC addresses: %w", err)
	}

	return &config, nil
}

// CalculateMACs calculates and sets MAC addresses for all network interfaces
// based on their IP addresses. This must be called after validation.
func (c *VMConfig) CalculateMACs() error {
	for i := range c.Network {
		mac, err := calculateMACFromIP(c.Network[i].IP)
		if err != nil {
			return fmt.Errorf("network_interfaces[%d]: %w", i, err)
		}
		c.Network[i].MACAddress = mac
	}
	return nil
}

// calculateMACFromIP generates a MAC address from an IP address.
// Algorithm (matching Ansible implementation from homestead/roles/libvirt-create):
//
//	IP: 10.20.30.40 → Octets: [10, 20, 30, 40] → Hex: [0a, 14, 1e, 28]
//	MAC: be:ef:0a:14:1e:28
//
// This ensures:
//   - Deterministic MAC addresses from IP
//   - Unique MACs per interface
//   - Compatibility with existing homestead VMs
func calculateMACFromIP(ipWithCIDR string) (string, error) {
	// Strip CIDR suffix if present
	ipStr := ipWithCIDR
	if strings.Contains(ipWithCIDR, "/") {
		ip, _, err := net.ParseCIDR(ipWithCIDR)
		if err != nil {
			return "", fmt.Errorf("invalid IP/CIDR format: %w", err)
		}
		ipStr = ip.String()
	}

	// Parse IP address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// Convert to IPv4 (net.ParseIP returns 16-byte form for IPv4)
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported: %s", ipStr)
	}

	// Generate MAC: be:ef:xx:xx:xx:xx where xx are the IP octets in hex
	mac := fmt.Sprintf("be:ef:%02x:%02x:%02x:%02x", ip[0], ip[1], ip[2], ip[3])

	return mac, nil
}
