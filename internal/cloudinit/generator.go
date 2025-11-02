// Package cloudinit provides cloud-init configuration generation for VM provisioning.
//
// This package generates cloud-init configuration files (user-data, meta-data, network-config)
// following the official cloud-init NoCloud datasource specification.
//
// See https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html
package cloudinit

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jbweber/plow/internal/config"
)

// UserData represents the cloud-config user-data structure.
// This is marshaled to YAML and prefixed with "#cloud-config" header.
//
// See https://cloudinit.readthedocs.io/en/latest/explanation/format.html#cloud-config-data
type UserData struct {
	Hostname          string    `yaml:"hostname"`
	FQDN              string    `yaml:"fqdn"`
	SSHAuthorizedKeys []string  `yaml:"ssh_authorized_keys,omitempty"`
	Chpasswd          *Chpasswd `yaml:"chpasswd,omitempty"`
	SSHPasswordAuth   bool      `yaml:"ssh_pwauth"`
	Output            *Output   `yaml:"output,omitempty"`
}

// Chpasswd configures user password settings.
type Chpasswd struct {
	Expire bool   `yaml:"expire"` // Whether to expire passwords on first login
	List   string `yaml:"list"`   // Format: "username:hash"
}

// Output configures cloud-init output logging.
type Output struct {
	All string `yaml:"all"`
}

// MetaData represents the cloud-init meta-data structure.
//
// See https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html
type MetaData struct {
	InstanceID    string `yaml:"instance-id"`
	LocalHostname string `yaml:"local-hostname"`
}

// NetworkConfig represents the netplan v2 network configuration.
//
// See https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html
type NetworkConfig struct {
	Version   int                       `yaml:"version"`
	Ethernets map[string]EthernetConfig `yaml:"ethernets"`
}

// EthernetConfig represents a single ethernet interface configuration.
type EthernetConfig struct {
	Match       MatchConfig   `yaml:"match"`
	Addresses   []string      `yaml:"addresses"`
	Routes      []RouteConfig `yaml:"routes,omitempty"`
	Nameservers *Nameservers  `yaml:"nameservers,omitempty"`
}

// MatchConfig matches an interface by MAC address.
type MatchConfig struct {
	MACAddress string `yaml:"macaddress"`
}

// RouteConfig represents a static route.
type RouteConfig struct {
	To  string `yaml:"to"`
	Via string `yaml:"via"`
}

// Nameservers represents DNS server configuration.
type Nameservers struct {
	Addresses []string `yaml:"addresses"`
}

// GenerateUserData generates the user-data YAML content from VM configuration.
//
// Returns the complete user-data file content including the "#cloud-config" header.
func GenerateUserData(cfg *config.VMConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("VM configuration cannot be nil")
	}

	// Derive hostname from FQDN or VM name
	hostname := cfg.Name
	fqdn := cfg.Name
	if cfg.CloudInit != nil && cfg.CloudInit.FQDN != "" {
		fqdn = cfg.CloudInit.FQDN
		// Extract hostname from FQDN (everything before first dot)
		hostname = strings.SplitN(fqdn, ".", 2)[0]
	}

	userData := UserData{
		Hostname:        hostname,
		FQDN:            fqdn,
		SSHPasswordAuth: false,
		Output: &Output{
			All: "| tee -a /var/log/cloud-init-output.log",
		},
	}

	// Add SSH keys if cloud-init is configured
	if cfg.CloudInit != nil {
		if len(cfg.CloudInit.SSHKeys) > 0 {
			userData.SSHAuthorizedKeys = cfg.CloudInit.SSHKeys
		}

		// Add root password hash if provided
		if cfg.CloudInit.RootPasswordHash != "" {
			userData.Chpasswd = &Chpasswd{
				Expire: false,
				List:   fmt.Sprintf("root:%s", cfg.CloudInit.RootPasswordHash),
			}
		}

		// Set SSH password authentication
		if cfg.CloudInit.SSHPwAuth != nil {
			userData.SSHPasswordAuth = *cfg.CloudInit.SSHPwAuth
		}
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(&userData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user-data to YAML: %w", err)
	}

	// Prepend #cloud-config header (required by cloud-init spec)
	return "#cloud-config\n" + string(yamlBytes), nil
}

// GenerateMetaData generates the meta-data YAML content from VM configuration.
//
// The instance-id is set to the VM name. Cloud-init uses instance-id to determine
// if this is a first boot. Using the VM name means cloud-init will re-run if the
// VM is destroyed and recreated with the same name.
func GenerateMetaData(cfg *config.VMConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("VM configuration cannot be nil")
	}

	metaData := MetaData{
		InstanceID:    cfg.Name,
		LocalHostname: cfg.Name,
	}

	yamlBytes, err := yaml.Marshal(&metaData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal meta-data to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// GenerateNetworkConfig generates the network-config YAML content from VM configuration.
//
// Uses netplan version 2 format with ethernet interfaces matched by MAC address.
//
// See https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html
func GenerateNetworkConfig(cfg *config.VMConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("VM configuration cannot be nil")
	}

	if len(cfg.Network) == 0 {
		return "", fmt.Errorf("at least one network interface is required")
	}

	networkConfig := NetworkConfig{
		Version:   2,
		Ethernets: make(map[string]EthernetConfig),
	}

	for i, iface := range cfg.Network {
		ethName := fmt.Sprintf("eth%d", i)

		ethConfig := EthernetConfig{
			Match: MatchConfig{
				MACAddress: iface.MACAddress,
			},
			Addresses: []string{iface.IP},
		}

		// Add default route if this interface should have one
		if iface.DefaultRoute {
			ethConfig.Routes = []RouteConfig{
				{
					To:  "0.0.0.0/0",
					Via: iface.Gateway,
				},
			}
		}

		// Add DNS servers if configured
		if len(iface.DNSServers) > 0 {
			ethConfig.Nameservers = &Nameservers{
				Addresses: iface.DNSServers,
			}
		}

		networkConfig.Ethernets[ethName] = ethConfig
	}

	yamlBytes, err := yaml.Marshal(&networkConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal network-config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
