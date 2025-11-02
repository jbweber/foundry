package libvirt

import (
	"fmt"
	"path/filepath"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/internal/config"
)

const (
	// BaseStoragePath is the default base path for VM storage
	BaseStoragePath = "/var/lib/libvirt/images"
)

// GenerateDomainXML generates libvirt domain XML from VM configuration
func GenerateDomainXML(cfg *config.VMConfig) (string, error) {
	domain := &libvirtxml.Domain{
		Type: "kvm",
		Name: cfg.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: uint(cfg.MemoryGiB),
			Unit:  "GiB",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     uint(cfg.VCPUs),
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
			Mode: "host-model", // Default CPU mode
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

	// Add boot disk
	bootDiskPath := filepath.Join(BaseStoragePath, cfg.GetBootDiskPath())
	bootDisk := libvirtxml.DomainDisk{
		Device: "disk",
		Driver: &libvirtxml.DomainDiskDriver{
			Name:  "qemu",
			Type:  "qcow2",
			Cache: "none",
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: bootDiskPath,
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

	// Add data disks
	for _, dataDisk := range cfg.DataDisks {
		diskPath := filepath.Join(BaseStoragePath, cfg.GetDataDiskPath(dataDisk.Device))
		disk := libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name:  "qemu",
				Type:  "qcow2",
				Cache: "none",
			},
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{
					File: diskPath,
				},
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: dataDisk.Device,
				Bus: "virtio",
			},
		}
		domain.Devices.Disks = append(domain.Devices.Disks, disk)
	}

	// Add cloud-init ISO if configured
	if cfg.CloudInit != nil {
		cloudInitPath := filepath.Join(BaseStoragePath, cfg.GetCloudInitISOPath())
		cdrom := libvirtxml.DomainDisk{
			Device: "cdrom",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: "raw",
			},
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{
					File: cloudInitPath,
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
	for _, iface := range cfg.Network {
		netIface := libvirtxml.DomainInterface{
			MAC: &libvirtxml.DomainInterfaceMAC{
				Address: iface.MACAddress,
			},
			Source: &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{
					Bridge: iface.Bridge,
				},
			},
			Model: &libvirtxml.DomainInterfaceModel{
				Type: "virtio",
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
