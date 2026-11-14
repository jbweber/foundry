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
	"strconv"
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

// VolumeNameInput returns the volume name for one of a VM's input disks
// (spec.inputDisks), by its ISO-9660 label. Labels are A-Z 0-9 _, so the
// lower-cased label names the volume without clashing with another label.
// Format: {vmName}_input-{label in lower case}.iso (e.g., "web-server_input-nodecfg.iso")
func VolumeNameInput(vmName, label string) string {
	return fmt.Sprintf("%s_input-%s.iso", vmName, strings.ToLower(label))
}

// MaxInputDisks is the number of input disks a VM can have: their CD-ROMs
// take the targets sdb to sdz (sda is the cloud-init ISO's).
const MaxInputDisks = 25

// InputDiskTarget returns the target device of the i-th input disk (from 0,
// below MaxInputDisks): sdb, sdc, ... sdz. On x86_64 the first SATA
// controller has six ports and libvirt adds another controller for the
// disks beyond them.
func InputDiskTarget(i int) string {
	return "sd" + string(rune('b'+i))
}

// VarstorePrefix starts the name of every Secure Boot key profile volume.
const VarstorePrefix = "varstore_"

// VolumeNameVarstore returns the volume name of a Secure Boot key profile's
// varstore for a guest architecture (libvirt's name, e.g. "x86_64").
// Format: varstore_{profile}_{arch}.fd (e.g., "varstore_my-keys_x86_64.fd")
func VolumeNameVarstore(profile, arch string) string {
	return fmt.Sprintf("%s%s_%s.fd", VarstorePrefix, profile, arch)
}

// ParseVolumeNameVarstore splits a varstore volume name into its profile and
// architecture; ok is false for any other volume name. Profile names have no
// "_" (an architecture may: "x86_64"), so the first "_" ends the profile.
func ParseVolumeNameVarstore(volume string) (profile, arch string, ok bool) {
	rest, found := strings.CutPrefix(volume, VarstorePrefix)
	if !found {
		return "", "", false
	}
	rest, found = strings.CutSuffix(rest, ".fd")
	if !found {
		return "", "", false
	}
	profile, arch, found = strings.Cut(rest, "_")
	if !found || profile == "" || arch == "" {
		return "", "", false
	}
	return profile, arch, true
}

// serialLogSuffix ends the name of every VM's serial console log.
const serialLogSuffix = "-serial.log"

// SerialLogName returns the name of a VM's serial console log, a file (and
// volume) of the logs pool. Not {vmName}.log: libvirt's own QEMU log for the
// domain has that name.
// Format: {vmName}-serial.log
func SerialLogName(vmName string) string {
	return vmName + serialLogSuffix
}

// SerialLogCopy reports whether volume is one of the VM's serial console log
// files: the log itself (copy -1), or one of the copies virtlogd rotates it
// into when it reaches its maximum size, {log}.0 (the newest) to {log}.N
// (copy N). Only these exact names match: "web-serial.log.bak", or the log
// of a VM "web-2", is not one of VM "web"'s.
func SerialLogCopy(volume, vmName string) (copyNumber int, ok bool) {
	base := SerialLogName(vmName)
	if volume == base {
		return -1, true
	}
	// virtlogd names the copies "%s.%zu": decimal, no sign, no leading zero
	rest, found := strings.CutPrefix(volume, base+".")
	if !found || rest == "" || strings.Trim(rest, "0123456789") != "" || (len(rest) > 1 && rest[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}
