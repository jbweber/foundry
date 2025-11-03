package libvirt

import (
	"fmt"
	"net"
	"strings"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

const (
	// BaseStoragePath is the default base path for VM storage
	BaseStoragePath = "/var/lib/libvirt/images"
)

// GetStoragePool returns the storage pool name, using default if not set.
func GetStoragePool(vm *v1alpha1.VirtualMachine) string {
	if vm.Spec.StoragePool == "" {
		return "foundry-vms"
	}
	return vm.Spec.StoragePool
}

// GetBootVolumeName returns the volume name for the boot disk.
// Format: <vm-name>_boot.qcow2
func GetBootVolumeName(vm *v1alpha1.VirtualMachine) string {
	return fmt.Sprintf("%s_boot.qcow2", vm.Name)
}

// GetDataVolumeName returns the volume name for a data disk.
// Format: <vm-name>_data-<device>.qcow2
func GetDataVolumeName(vm *v1alpha1.VirtualMachine, device string) string {
	return fmt.Sprintf("%s_data-%s.qcow2", vm.Name, device)
}

// GetCloudInitVolumeName returns the volume name for the cloud-init ISO.
// Format: <vm-name>_cloudinit.iso
func GetCloudInitVolumeName(vm *v1alpha1.VirtualMachine) string {
	return fmt.Sprintf("%s_cloudinit.iso", vm.Name)
}

// calculateMACFromIP generates a MAC address from an IP address.
// Algorithm: IP 10.20.30.40 → MAC be:ef:0a:14:1e:28
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

	// Convert to IPv4
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported: %s", ipStr)
	}

	// Generate MAC: be:ef:xx:xx:xx:xx
	return fmt.Sprintf("be:ef:%02x:%02x:%02x:%02x", ip[0], ip[1], ip[2], ip[3]), nil
}

// calculateInterfaceNameFromIP generates a tap interface name from an IP address.
// Algorithm: IP 10.20.30.40 → Interface vm0a141e28
func calculateInterfaceNameFromIP(ipWithCIDR string) (string, error) {
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

	// Convert to IPv4
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported: %s", ipStr)
	}

	// Generate interface name: vm{hex octets}
	return fmt.Sprintf("vm%02x%02x%02x%02x", ip[0], ip[1], ip[2], ip[3]), nil
}

// GenerateDomainXML generates libvirt domain XML from VM configuration
func GenerateDomainXML(vm *v1alpha1.VirtualMachine) (string, error) {
	// Get CPU mode with default
	cpuMode := vm.Spec.CPUMode
	if cpuMode == "" {
		cpuMode = "host-model"
	}

	domain := &libvirtxml.Domain{
		Type: "kvm",
		Name: vm.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: uint(vm.Spec.MemoryGiB),
			Unit:  "GiB",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     uint(vm.Spec.VCPUs),
		},
		OS: &libvirtxml.DomainOS{
			Firmware: "efi",
			Type: &libvirtxml.DomainOSType{
				Arch: "x86_64",
				Type: "hvm",
			},
			BIOS: &libvirtxml.DomainBIOS{
				UseSerial: "yes",
			},
		},
		Features: &libvirtxml.DomainFeatureList{
			ACPI: &libvirtxml.DomainFeature{},
			APIC: &libvirtxml.DomainFeatureAPIC{},
			PAE:  &libvirtxml.DomainFeature{},
		},
		CPU: &libvirtxml.DomainCPU{
			Mode: cpuMode,
			Model: &libvirtxml.DomainCPUModel{
				Fallback: "allow",
			},
		},
		Clock: &libvirtxml.DomainClock{
			Offset: "utc",
			Timer: []libvirtxml.DomainTimer{
				{Name: "rtc", TickPolicy: "catchup"},
				{Name: "pit", TickPolicy: "delay"},
				{Name: "hpet", Present: "no"},
			},
		},
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "restart",
		Devices: &libvirtxml.DomainDeviceList{
			Controllers: []libvirtxml.DomainController{
				{
					Type:  "pci",
					Index: func() *uint { i := uint(0); return &i }(),
					Model: "pci-root",
				},
			},
			MemBalloon: &libvirtxml.DomainMemBalloon{
				Model: "virtio",
			},
			RNGs: []libvirtxml.DomainRNG{
				{
					Model: "virtio",
					Backend: &libvirtxml.DomainRNGBackend{
						Random: &libvirtxml.DomainRNGBackendRandom{
							Device: "/dev/urandom",
						},
					},
				},
			},
		},
	}

	// Add boot disk (volume-based)
	bootDisk := libvirtxml.DomainDisk{
		Device: "disk",
		Driver: &libvirtxml.DomainDiskDriver{
			Name:  "qemu",
			Type:  "qcow2",
			Cache: "none",
		},
		Source: &libvirtxml.DomainDiskSource{
			Volume: &libvirtxml.DomainDiskSourceVolume{
				Pool:   GetStoragePool(vm),
				Volume: GetBootVolumeName(vm),
			},
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "vda",
			Bus: "virtio",
		},
		Boot: &libvirtxml.DomainDeviceBoot{
			Order: 1,
		},
	}
	domain.Devices.Disks = append(domain.Devices.Disks, bootDisk)

	// Add data disks (volume-based)
	for _, dataDisk := range vm.Spec.DataDisks {
		disk := libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name:  "qemu",
				Type:  "qcow2",
				Cache: "none",
			},
			Source: &libvirtxml.DomainDiskSource{
				Volume: &libvirtxml.DomainDiskSourceVolume{
					Pool:   GetStoragePool(vm),
					Volume: GetDataVolumeName(vm, dataDisk.Device),
				},
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: dataDisk.Device,
				Bus: "virtio",
			},
		}
		domain.Devices.Disks = append(domain.Devices.Disks, disk)
	}

	// Add cloud-init ISO if configured (volume-based)
	if vm.Spec.CloudInit != nil {
		cdrom := libvirtxml.DomainDisk{
			Device: "cdrom",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: "raw",
			},
			Source: &libvirtxml.DomainDiskSource{
				Volume: &libvirtxml.DomainDiskSourceVolume{
					Pool:   GetStoragePool(vm),
					Volume: GetCloudInitVolumeName(vm),
				},
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: "sda",
				Bus: "sata",
			},
			ReadOnly: &libvirtxml.DomainDiskReadOnly{},
		}
		domain.Devices.Disks = append(domain.Devices.Disks, cdrom)
	}

	// Add network interfaces
	for _, iface := range vm.Spec.NetworkInterfaces {
		// Calculate MAC address from IP
		macAddr, err := calculateMACFromIP(iface.IP)
		if err != nil {
			return "", fmt.Errorf("failed to calculate MAC address for %s: %w", iface.IP, err)
		}

		// Calculate interface name from IP
		ifaceName, err := calculateInterfaceNameFromIP(iface.IP)
		if err != nil {
			return "", fmt.Errorf("failed to calculate interface name for %s: %w", iface.IP, err)
		}

		netIface := libvirtxml.DomainInterface{
			MAC: &libvirtxml.DomainInterfaceMAC{
				Address: macAddr,
			},
			Source: &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{
					Bridge: iface.Bridge,
				},
			},
			Model: &libvirtxml.DomainInterfaceModel{
				Type: "virtio",
			},
			Target: &libvirtxml.DomainInterfaceTarget{
				Dev: ifaceName,
			},
		}
		domain.Devices.Interfaces = append(domain.Devices.Interfaces, netIface)
	}

	// Add serial console
	domain.Devices.Serials = []libvirtxml.DomainSerial{
		{
			Source: &libvirtxml.DomainChardevSource{
				Pty: &libvirtxml.DomainChardevSourcePty{},
			},
			Target: &libvirtxml.DomainSerialTarget{
				Port: func() *uint { p := uint(0); return &p }(),
			},
		},
	}
	domain.Devices.Consoles = []libvirtxml.DomainConsole{
		{
			Source: &libvirtxml.DomainChardevSource{
				Pty: &libvirtxml.DomainChardevSourcePty{},
			},
			Target: &libvirtxml.DomainConsoleTarget{
				Type: "serial",
				Port: func() *uint { p := uint(0); return &p }(),
			},
		},
	}

	// Marshal to XML
	xml, err := domain.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal domain XML: %w", err)
	}

	return xml, nil
}
