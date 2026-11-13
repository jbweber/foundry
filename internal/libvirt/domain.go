package libvirt

import (
	"fmt"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/naming"
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
	return naming.VolumeNameBoot(vm.Name)
}

// GetDataVolumeName returns the volume name for a data disk.
// Format: <vm-name>_data-<device>.qcow2
func GetDataVolumeName(vm *v1alpha1.VirtualMachine, device string) string {
	return naming.VolumeNameData(vm.Name, device)
}

// GetCloudInitVolumeName returns the volume name for the cloud-init ISO.
// Format: <vm-name>_cloudinit.iso
func GetCloudInitVolumeName(vm *v1alpha1.VirtualMachine) string {
	return naming.VolumeNameCloudInit(vm.Name)
}

// GenerateDomainXML generates libvirt domain XML from VM configuration.
//
// hostArch is the architecture of the machine libvirt runs on, in libvirt's
// names (see HostArch). A guest of that architecture runs under KVM, any
// other under QEMU emulation. A spec without spec.arch is an x86_64 guest
// under KVM whatever hostArch is, as before spec.arch existed.
//
// x86_64 guests use the q35 machine; aarch64 guests use the virt machine,
// with the differences applied by applyAarch64.
func GenerateDomainXML(vm *v1alpha1.VirtualMachine, hostArch string) (string, error) {
	domainType := vm.GetDomainType(hostArch)
	cpuMode := vm.CPUModeFor(domainType)

	// host-model and host-passthrough describe the host's CPU, which an
	// emulated guest of another architecture cannot have; libvirt would
	// refuse the domain at start time.
	if domainType == v1alpha1.DomainTypeQEMU && cpuMode != v1alpha1.CPUModeMaximum {
		return "", fmt.Errorf("spec.cpuMode %s is not supported for an emulated %s guest on a %s host; use %s or leave it unset",
			cpuMode, vm.GetArch(), hostArch, v1alpha1.CPUModeMaximum)
	}

	domain := &libvirtxml.Domain{
		Type: domainType,
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
				Arch:    "x86_64",
				Machine: "q35",
				Type:    "hvm",
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
					Model: "pcie-root",
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

	// Determine boot order based on PXE boot configuration
	// If any interface has PXEBoot enabled, network boots first (order 1),
	// then disk (order 2). Otherwise, disk boots first (order 1).
	hasPXEBoot := false
	for _, iface := range vm.Spec.NetworkInterfaces {
		if iface.PXEBoot {
			hasPXEBoot = true
			break
		}
	}

	diskBootOrder := uint(1)
	if hasPXEBoot {
		diskBootOrder = 2
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
			Order: diskBootOrder,
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

	if vm.GetArch() == v1alpha1.ArchAarch64 {
		applyAarch64(domain)
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
		addCdrom(domain, cdrom)
	}

	// Add network interfaces
	for _, iface := range vm.Spec.NetworkInterfaces {
		// Calculate MAC address from IP
		macAddr, err := naming.MACFromIP(iface.IP)
		if err != nil {
			return "", fmt.Errorf("failed to calculate MAC address for %s: %w", iface.IP, err)
		}

		// Calculate interface name from IP
		ifaceName, err := naming.InterfaceNameFromIP(iface.IP)
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

		if iface.Vlan != nil {
			netIface.VLan = &libvirtxml.DomainInterfaceVLan{
				Tags: []libvirtxml.DomainInterfaceVLanTag{
					{ID: *iface.Vlan},
				},
			}
		}

		// Add boot order if PXE boot is enabled for this interface
		if iface.PXEBoot {
			netIface.Boot = &libvirtxml.DomainDeviceBoot{
				Order: 1,
			}
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

// addCdrom adds a CD-ROM to a domain, on the bus its machine has: SATA on
// x86_64 (q35). The aarch64 virt machine has no AHCI controller, so SATA is
// unavailable there; it gets a SCSI cdrom on a virtio-scsi controller, as
// virt-install does for every non-x86 guest that supports virtio-scsi:
// https://github.com/virt-manager/virt-manager/blob/main/virtinst/devices/disk.py (_default_bus)
func addCdrom(domain *libvirtxml.Domain, cdrom libvirtxml.DomainDisk) {
	if domain.OS.Type.Arch == v1alpha1.ArchAarch64 {
		cdrom.Target.Bus = "scsi"
		domain.Devices.Controllers = append(domain.Devices.Controllers, libvirtxml.DomainController{
			Type:  "scsi",
			Index: func() *uint { i := uint(0); return &i }(),
			Model: "virtio-scsi",
		})
	}
	domain.Devices.Disks = append(domain.Devices.Disks, cdrom)
}

// applyAarch64 adjusts an x86_64 (q35) domain definition for an aarch64 guest
// on QEMU's virt machine. Everything not touched here (UEFI firmware
// auto-selection, pcie-root, virtio devices, serial console) is common to
// both: libvirt picks the AAVMF firmware for aarch64 as it picks OVMF for
// x86_64 (https://libvirt.org/formatdomain.html#bios-bootloader).
//
// The serial device keeps no explicit target type: libvirt picks
// system-serial (pl011) for aarch64/virt, as it picks isa-serial for x86_64
// (https://libvirt.org/formatdomain.html#consoles-serial-parallel-channel-devices).
// The guest's console is ttyAMA0 instead of ttyS0.
func applyAarch64(domain *libvirtxml.Domain) {
	domain.OS.Type.Arch = v1alpha1.ArchAarch64
	domain.OS.Type.Machine = "virt"

	// <bios useserial> drives the x86 Serial Graphics Adapter; libvirt rejects
	// it on other architectures ("BIOS serial console only supported on x86
	// architectures", qemuValidateDomainDef in
	// https://gitlab.com/libvirt/libvirt/-/blob/master/src/qemu/qemu_validate.c).
	domain.OS.BIOS = nil

	// Arm uses a GIC rather than an APIC, and has no PAE. GICv3 works under
	// both KVM on current Arm servers and TCG; "host" would fail under TCG
	// (https://libvirt.org/formatdomain.html#hypervisor-features).
	domain.Features = &libvirtxml.DomainFeatureList{
		ACPI: &libvirtxml.DomainFeature{},
		GIC:  &libvirtxml.DomainFeatureGIC{Version: "3"},
	}

	// The model fallback only matters for x86 named CPU models.
	domain.CPU.Model = nil

	// rtc, pit and hpet are x86 timers; the Arm guest uses the architected
	// generic timer, which needs no configuration.
	domain.Clock.Timer = nil
}
