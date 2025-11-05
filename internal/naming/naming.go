// Package naming provides infrastructure-level naming conventions for
// libvirt resources. This includes MAC address calculation from IP,
// interface naming, and volume naming patterns.
//
// These naming rules are version-independent and shared across all
// API versions.
package naming

import (
	"fmt"
	"net"
	"strings"
)

// MACFromIP calculates a deterministic MAC address from an IP address.
// Uses the RFC 2731 local assignment prefix be:ef:.
//
// Example: IP 10.55.22.22 → MAC be:ef:0a:37:16:16
func MACFromIP(ip string) (string, error) {
	// Parse IP (handles both "10.1.2.3" and "10.1.2.3/24")
	ipStr := ip
	if strings.Contains(ip, "/") {
		ipAddr, _, err := net.ParseCIDR(ip)
		if err != nil {
			return "", fmt.Errorf("invalid IP/CIDR: %w", err)
		}
		ipStr = ipAddr.String()
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// Get IPv4 representation
	ipv4 := parsedIP.To4()
	if ipv4 == nil {
		return "", fmt.Errorf("not an IPv4 address: %s", ipStr)
	}

	// Format: be:ef:XX:XX:XX:XX where XX are IP octets in hex
	return fmt.Sprintf("be:ef:%02x:%02x:%02x:%02x",
		ipv4[0], ipv4[1], ipv4[2], ipv4[3]), nil
}

// InterfaceNameFromIP calculates a deterministic tap interface name from an IP address.
// Format: vm{hex_octets} (10 chars total, well within Linux 15-char limit)
//
// Example: IP 10.55.22.22 → vm0a371616
func InterfaceNameFromIP(ip string) (string, error) {
	// Parse IP (handles both "10.1.2.3" and "10.1.2.3/24")
	ipStr := ip
	if strings.Contains(ip, "/") {
		ipAddr, _, err := net.ParseCIDR(ip)
		if err != nil {
			return "", fmt.Errorf("invalid IP/CIDR: %w", err)
		}
		ipStr = ipAddr.String()
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// Get IPv4 representation
	ipv4 := parsedIP.To4()
	if ipv4 == nil {
		return "", fmt.Errorf("not an IPv4 address: %s", ipStr)
	}

	// Format: vm{8 hex digits}
	return fmt.Sprintf("vm%02x%02x%02x%02x",
		ipv4[0], ipv4[1], ipv4[2], ipv4[3]), nil
}

// VolumeNameBoot returns the volume name for a VM's boot disk.
// Format: {vmName}_boot.qcow2
func VolumeNameBoot(vmName string) string {
	return fmt.Sprintf("%s_boot.qcow2", vmName)
}

// VolumeNameData returns the volume name for a VM's data disk.
// Format: {vmName}_data-{device}.qcow2 (e.g., "web-server_data-vdb.qcow2")
func VolumeNameData(vmName, device string) string {
	return fmt.Sprintf("%s_data-%s.qcow2", vmName, device)
}

// VolumeNameCloudInit returns the volume name for a VM's cloud-init ISO.
// Format: {vmName}_cloudinit.iso
func VolumeNameCloudInit(vmName string) string {
	return fmt.Sprintf("%s_cloudinit.iso", vmName)
}
