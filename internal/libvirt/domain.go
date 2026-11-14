package libvirt

import (
	"fmt"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/naming"
	"github.com/jbweber/foundry/internal/storage"
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
	if vm.Spec.Firmware != nil {
		applyFirmware(domain, vm.Spec.Firmware, vm.GetArch())
	}
	if vm.HasSMBIOS() {
		applySMBIOS(domain, vm)
	}
	addFwCfg(domain, vm.Spec.FwCfg)

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

// applyFirmware replaces UEFI auto-selection (firmware='efi') with an explicit
// pflash loader and, if given, the varstore template, so the VM gets exactly
// the firmware and enrolled keys asked for. libvirt copies the template to the
// VM's own varstore (/var/lib/libvirt/qemu/nvram/<name>_VARS.fd) on first
// start and keeps it until the domain is undefined with its NVRAM, as
// destroy does (https://libvirt.org/formatdomain.html#bios-bootloader).
//
// A secure loader lets only SMM code write the varstore, which is what keeps
// Secure Boot's keys out of the guest OS's reach; libvirt refuses
// secure='yes' without <smm state='on'/>
// (https://libvirt.org/formatdomain.html#hypervisor-features). SMM exists
// only on x86, so the loader validates secureBoot as x86_64 only.
func applyFirmware(domain *libvirtxml.Domain, fw *v1alpha1.FirmwareSpec, arch string) {
	domain.OS.Firmware = ""
	domain.OS.Loader = &libvirtxml.DomainLoader{
		Path:     fw.Loader,
		Readonly: "yes",
		Type:     "pflash",
	}
	if template := nvramTemplate(fw, arch); template != "" {
		domain.OS.NVRam = &libvirtxml.DomainNVRam{Template: template}
	}
	if fw.SecureBoot {
		domain.OS.Loader.Secure = "yes"
		domain.Features.SMM = &libvirtxml.DomainFeatureSMM{State: "on"}
	}
}

// nvramTemplate returns the varstore template for a firmware spec: the
// nvramTemplate path, or for a key profile (spec.firmware.keys) the path its
// varstore volume has in the default foundry-images pool. create does not
// rely on that default: it asks libvirt for the volume's path and passes it
// as NVRAMTemplate (resolveKeyProfile in internal/vm), so this path is what
// render shows and what create gets with the default pool layout.
//
// libvirt takes the template only as a host path, not as a pool volume
// (https://libvirt.org/formatdomain.html#bios-bootloader).
func nvramTemplate(fw *v1alpha1.FirmwareSpec, arch string) string {
	if fw.NVRAMTemplate != "" || fw.Keys == "" {
		return fw.NVRAMTemplate
	}
	return storage.DefaultImagesPath + "/" + naming.VolumeNameVarstore(fw.Keys, arch)
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

// applySMBIOS sets the SMBIOS system information (type 1) through libvirt's
// sysinfo (https://libvirt.org/formatdomain.html#smbios-system-information):
// <os><smbios mode='sysinfo'/> takes the values from <sysinfo type='smbios'>.
// The serial is always among them, stable per VM, so a guest can bind to it.
// libvirt requires the domain UUID and the SMBIOS one to match, so a set
// spec.smbios.uuid becomes the domain's UUID too.
func applySMBIOS(domain *libvirtxml.Domain, vm *v1alpha1.VirtualMachine) {
	domain.OS.SMBios = &libvirtxml.DomainSMBios{Mode: "sysinfo"}
	domain.SysInfo = append(domain.SysInfo, libvirtxml.DomainSysInfo{SMBIOS: &libvirtxml.DomainSysInfoSMBIOS{
		System: &libvirtxml.DomainSysInfoSystem{Entry: smbiosSystemEntries(vm)},
	}})
	if s := vm.Spec.SMBIOS; s != nil && s.UUID != "" {
		domain.UUID = s.UUID
	}
}

// smbiosSystemEntries returns the <sysinfo type='smbios'><system> entries:
// the serial, and whatever spec.smbios sets, in the order libvirt documents
// them.
func smbiosSystemEntries(vm *v1alpha1.VirtualMachine) []libvirtxml.DomainSysInfoEntry {
	s := vm.Spec.SMBIOS
	if s == nil {
		s = &v1alpha1.SMBIOSSpec{}
	}
	var entries []libvirtxml.DomainSysInfoEntry
	for _, e := range []libvirtxml.DomainSysInfoEntry{
		{Name: "manufacturer", Value: s.Manufacturer},
		{Name: "product", Value: s.Product},
		{Name: "version", Value: s.Version},
		{Name: "serial", Value: vm.GetSerial()},
		{Name: "uuid", Value: s.UUID},
		{Name: "sku", Value: s.SKU},
		{Name: "family", Value: s.Family},
	} {
		if e.Value != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// addFwCfg adds fw_cfg entries through libvirt's native <sysinfo type='fwcfg'>
// (https://libvirt.org/formatdomain.html#smbios-system-information). The
// guest reads them from /sys/firmware/qemu_fw_cfg/by_name/<name>/raw.
func addFwCfg(domain *libvirtxml.Domain, entries []v1alpha1.FwCfgEntrySpec) {
	if len(entries) == 0 {
		return
	}
	fwcfg := &libvirtxml.DomainSysInfoFWCfg{}
	for _, e := range entries {
		fwcfg.Entry = append(fwcfg.Entry, libvirtxml.DomainSysInfoEntry{
			Name:  e.Name,
			File:  e.File,
			Value: e.String,
		})
	}
	domain.SysInfo = append(domain.SysInfo, libvirtxml.DomainSysInfo{FWCfg: fwcfg})
}
