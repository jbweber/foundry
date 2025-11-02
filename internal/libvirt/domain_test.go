package libvirt

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/internal/config"
)

func TestGenerateDomainXML(t *testing.T) {
	tests := []struct {
		name    string
		vmCfg   *config.VMConfig
		wantErr bool
	}{
		{
			name: "simple VM with cloud-init",
			vmCfg: &config.VMConfig{
				Name:      "test-vm",
				VCPUs:     4,
				MemoryGiB: 8,
				BootDisk: config.BootDiskConfig{
					SizeGB: 50,
					Image:  "/var/lib/libvirt/images/fedora-42.qcow2",
				},
				Network: []config.NetworkInterface{
					{
						IP:         "10.20.30.40/24",
						Gateway:    "10.20.30.1",
						DNSServers: []string{"8.8.8.8", "1.1.1.1"},
						Bridge:     "br0",
						MACAddress: "be:ef:0a:14:1e:28",
					},
				},
				CloudInit: &config.CloudInitConfig{
					FQDN:    "test-vm.example.com",
					SSHKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo test@example.com"},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with multiple data disks",
			vmCfg: &config.VMConfig{
				Name:      "multi-disk-vm",
				VCPUs:     2,
				MemoryGiB: 4,
				BootDisk: config.BootDiskConfig{
					SizeGB: 30,
					Image:  "/var/lib/libvirt/images/ubuntu-24.04.qcow2",
				},
				DataDisks: []config.DataDiskConfig{
					{Device: "vdb", SizeGB: 100},
					{Device: "vdc", SizeGB: 200},
				},
				Network: []config.NetworkInterface{
					{
						IP:         "192.168.1.50/24",
						Gateway:    "192.168.1.1",
						Bridge:     "br1",
						MACAddress: "be:ef:c0:a8:01:32",
					},
				},
				CloudInit: &config.CloudInitConfig{
					FQDN: "multi-disk.local",
				},
			},
			wantErr: false,
		},
		{
			name: "VM without cloud-init",
			vmCfg: &config.VMConfig{
				Name:      "no-cloudinit-vm",
				VCPUs:     8,
				MemoryGiB: 16,
				BootDisk: config.BootDiskConfig{
					SizeGB: 100,
					Empty:  true,
				},
				Network: []config.NetworkInterface{
					{
						IP:         "10.55.22.22/24",
						Gateway:    "10.55.22.1",
						Bridge:     "br0",
						MACAddress: "be:ef:0a:37:16:16",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with multiple NICs",
			vmCfg: &config.VMConfig{
				Name:      "multi-nic-vm",
				VCPUs:     4,
				MemoryGiB: 8,
				BootDisk: config.BootDiskConfig{
					SizeGB: 50,
					Image:  "/var/lib/libvirt/images/base.qcow2",
				},
				Network: []config.NetworkInterface{
					{
						IP:         "10.20.30.40/24",
						Gateway:    "10.20.30.1",
						Bridge:     "br0",
						MACAddress: "be:ef:0a:14:1e:28",
					},
					{
						IP:         "192.168.1.100/24",
						Gateway:    "192.168.1.1",
						Bridge:     "br1",
						MACAddress: "be:ef:c0:a8:01:64",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize config to set default storage pools
			tt.vmCfg.Normalize()

			xml, err := GenerateDomainXML(tt.vmCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateDomainXML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Verify XML is not empty
			if xml == "" {
				t.Error("GenerateDomainXML() returned empty XML")
				return
			}

			// Verify XML can be parsed back
			var domain libvirtxml.Domain
			if err := domain.Unmarshal(xml); err != nil {
				t.Errorf("Generated XML cannot be unmarshaled: %v\nXML:\n%s", err, xml)
				return
			}

			// Validate basic structure matches Ansible reference format
			validateDomainStructure(t, &domain, tt.vmCfg)
		})
	}
}

// validateDomainStructure validates the domain XML structure matches Ansible reference
func validateDomainStructure(t *testing.T, domain *libvirtxml.Domain, cfg *config.VMConfig) {
	t.Helper()

	// Validate basic metadata
	if domain.Type != "kvm" {
		t.Errorf("domain type = %v, want kvm", domain.Type)
	}
	if domain.Name != cfg.Name {
		t.Errorf("domain name = %v, want %v", domain.Name, cfg.Name)
	}

	// Validate memory
	if domain.Memory == nil {
		t.Error("domain memory is nil")
	} else {
		if domain.Memory.Value != uint(cfg.MemoryGiB) {
			t.Errorf("memory value = %v, want %v", domain.Memory.Value, cfg.MemoryGiB)
		}
		if domain.Memory.Unit != "GiB" {
			t.Errorf("memory unit = %v, want GiB", domain.Memory.Unit)
		}
	}

	// Validate VCPUs
	if domain.VCPU == nil {
		t.Error("domain VCPU is nil")
	} else {
		if domain.VCPU.Value != uint(cfg.VCPUs) {
			t.Errorf("vcpu value = %v, want %v", domain.VCPU.Value, cfg.VCPUs)
		}
		if domain.VCPU.Placement != "static" {
			t.Errorf("vcpu placement = %v, want static", domain.VCPU.Placement)
		}
	}

	// Validate OS (UEFI firmware)
	if domain.OS == nil {
		t.Error("domain OS is nil")
	} else {
		if domain.OS.Firmware != "efi" {
			t.Errorf("OS firmware = %v, want efi", domain.OS.Firmware)
		}
		if domain.OS.Type == nil || domain.OS.Type.Arch != "x86_64" {
			t.Error("OS type arch should be x86_64")
		}
		if domain.OS.Type == nil || domain.OS.Type.Type != "hvm" {
			t.Error("OS type should be hvm")
		}
		if domain.OS.BIOS == nil || domain.OS.BIOS.UseSerial != "yes" {
			t.Error("BIOS useserial should be yes")
		}
	}

	// Validate features (ACPI, APIC, PAE)
	if domain.Features == nil {
		t.Error("domain features is nil")
	} else {
		if domain.Features.ACPI == nil {
			t.Error("ACPI feature missing")
		}
		if domain.Features.APIC == nil {
			t.Error("APIC feature missing")
		}
		if domain.Features.PAE == nil {
			t.Error("PAE feature missing")
		}
	}

	// Validate CPU
	if domain.CPU == nil {
		t.Error("domain CPU is nil")
	} else {
		if domain.CPU.Mode != "host-model" {
			t.Errorf("CPU mode = %v, want host-model", domain.CPU.Mode)
		}
		if domain.CPU.Model == nil || domain.CPU.Model.Fallback != "allow" {
			t.Error("CPU model fallback should be allow")
		}
	}

	// Validate clock
	if domain.Clock == nil {
		t.Error("domain clock is nil")
	} else {
		if domain.Clock.Offset != "utc" {
			t.Errorf("clock offset = %v, want utc", domain.Clock.Offset)
		}
		if len(domain.Clock.Timer) != 3 {
			t.Errorf("clock timers count = %v, want 3", len(domain.Clock.Timer))
		}
	}

	// Validate lifecycle events
	if domain.OnPoweroff != "destroy" {
		t.Errorf("on_poweroff = %v, want destroy", domain.OnPoweroff)
	}
	if domain.OnReboot != "restart" {
		t.Errorf("on_reboot = %v, want restart", domain.OnReboot)
	}
	if domain.OnCrash != "restart" {
		t.Errorf("on_crash = %v, want restart", domain.OnCrash)
	}

	// Validate devices
	if domain.Devices == nil {
		t.Fatal("domain devices is nil")
	}

	// Validate disks
	expectedDiskCount := 1 + len(cfg.DataDisks) // boot disk + data disks
	if cfg.CloudInit != nil {
		expectedDiskCount++ // cloud-init ISO
	}
	if len(domain.Devices.Disks) != expectedDiskCount {
		t.Errorf("disk count = %v, want %v", len(domain.Devices.Disks), expectedDiskCount)
	}

	// Validate boot disk
	if len(domain.Devices.Disks) > 0 {
		bootDisk := domain.Devices.Disks[0]
		if bootDisk.Device != "disk" {
			t.Errorf("boot disk device = %v, want disk", bootDisk.Device)
		}
		if bootDisk.Driver == nil || bootDisk.Driver.Type != "qcow2" {
			t.Error("boot disk driver type should be qcow2")
		}
		if bootDisk.Driver == nil || bootDisk.Driver.Cache != "none" {
			t.Error("boot disk cache should be none")
		}
		if bootDisk.Target == nil || bootDisk.Target.Dev != "vda" {
			t.Error("boot disk target should be vda")
		}
		if bootDisk.Target == nil || bootDisk.Target.Bus != "virtio" {
			t.Error("boot disk bus should be virtio")
		}
		if bootDisk.Boot == nil || bootDisk.Boot.Order != 1 {
			t.Error("boot disk should have boot order 1")
		}
		if bootDisk.Source == nil || bootDisk.Source.Volume == nil {
			t.Error("boot disk source volume is nil")
		} else {
			expectedPool := cfg.GetStoragePool()
			expectedVolume := cfg.GetBootVolumeName()
			if bootDisk.Source.Volume.Pool != expectedPool {
				t.Errorf("boot disk pool = %v, want %v", bootDisk.Source.Volume.Pool, expectedPool)
			}
			if bootDisk.Source.Volume.Volume != expectedVolume {
				t.Errorf("boot disk volume = %v, want %v", bootDisk.Source.Volume.Volume, expectedVolume)
			}
		}
	}

	// Validate data disks
	for i, dataDiskCfg := range cfg.DataDisks {
		diskIdx := i + 1 // boot disk is at index 0
		if len(domain.Devices.Disks) <= diskIdx {
			t.Errorf("data disk %v missing", dataDiskCfg.Device)
			continue
		}
		disk := domain.Devices.Disks[diskIdx]
		if disk.Device != "disk" {
			t.Errorf("data disk %v device = %v, want disk", dataDiskCfg.Device, disk.Device)
		}
		if disk.Target == nil || disk.Target.Dev != dataDiskCfg.Device {
			t.Errorf("data disk target = %v, want %v", disk.Target.Dev, dataDiskCfg.Device)
		}
		if disk.Source == nil || disk.Source.Volume == nil {
			t.Errorf("data disk %v source volume is nil", dataDiskCfg.Device)
		} else {
			expectedPool := cfg.GetStoragePool()
			expectedVolume := cfg.GetDataVolumeName(dataDiskCfg.Device)
			if disk.Source.Volume.Pool != expectedPool {
				t.Errorf("data disk pool = %v, want %v", disk.Source.Volume.Pool, expectedPool)
			}
			if disk.Source.Volume.Volume != expectedVolume {
				t.Errorf("data disk volume = %v, want %v", disk.Source.Volume.Volume, expectedVolume)
			}
		}
	}

	// Validate cloud-init ISO (if configured)
	if cfg.CloudInit != nil {
		cdromIdx := 1 + len(cfg.DataDisks)
		if len(domain.Devices.Disks) <= cdromIdx {
			t.Error("cloud-init CDROM missing")
		} else {
			cdrom := domain.Devices.Disks[cdromIdx]
			if cdrom.Device != "cdrom" {
				t.Errorf("cloud-init device = %v, want cdrom", cdrom.Device)
			}
			if cdrom.Driver == nil || cdrom.Driver.Type != "raw" {
				t.Error("cloud-init driver type should be raw")
			}
			if cdrom.Target == nil || cdrom.Target.Dev != "sda" {
				t.Error("cloud-init target should be sda")
			}
			if cdrom.Target == nil || cdrom.Target.Bus != "sata" {
				t.Error("cloud-init bus should be sata")
			}
			if cdrom.ReadOnly == nil {
				t.Error("cloud-init should be readonly")
			}
			// Cloud-init is now volume-based, not file-based
			if cdrom.Source == nil || cdrom.Source.Volume == nil {
				t.Error("cloud-init source is nil")
			} else {
				expectedPool := cfg.GetStoragePool()
				expectedVolume := cfg.GetCloudInitVolumeName()
				if cdrom.Source.Volume.Pool != expectedPool {
					t.Errorf("cloud-init pool = %v, want %v", cdrom.Source.Volume.Pool, expectedPool)
				}
				if cdrom.Source.Volume.Volume != expectedVolume {
					t.Errorf("cloud-init volume = %v, want %v", cdrom.Source.Volume.Volume, expectedVolume)
				}
			}
		}
	}

	// Validate network interfaces
	if len(domain.Devices.Interfaces) != len(cfg.Network) {
		t.Errorf("interface count = %v, want %v", len(domain.Devices.Interfaces), len(cfg.Network))
	}
	for i, ifaceCfg := range cfg.Network {
		if len(domain.Devices.Interfaces) <= i {
			t.Errorf("network interface %v missing", i)
			continue
		}
		iface := domain.Devices.Interfaces[i]
		// Note: interface type is determined by which Source union member is set
		if iface.Source == nil || iface.Source.Bridge == nil {
			t.Errorf("interface %v should have bridge source", i)
		}
		if iface.MAC == nil || iface.MAC.Address != ifaceCfg.MACAddress {
			t.Errorf("interface %v MAC = %v, want %v", i, iface.MAC.Address, ifaceCfg.MACAddress)
		}
		if iface.Source == nil || iface.Source.Bridge == nil || iface.Source.Bridge.Bridge != ifaceCfg.Bridge {
			t.Errorf("interface %v bridge = %v, want %v", i, iface.Source.Bridge.Bridge, ifaceCfg.Bridge)
		}
		if iface.Model == nil || iface.Model.Type != "virtio" {
			t.Errorf("interface %v model should be virtio", i)
		}
	}

	// Validate controllers
	if len(domain.Devices.Controllers) == 0 {
		t.Error("no controllers defined")
	} else {
		pciFound := false
		for _, ctrl := range domain.Devices.Controllers {
			if ctrl.Type == "pci" && ctrl.Model == "pci-root" {
				pciFound = true
				break
			}
		}
		if !pciFound {
			t.Error("pci-root controller not found")
		}
	}

	// Validate serial console
	if len(domain.Devices.Serials) == 0 {
		t.Error("no serial devices defined")
	} else {
		serial := domain.Devices.Serials[0]
		if serial.Source == nil || serial.Source.Pty == nil {
			t.Error("serial should have pty source")
		}
		if serial.Target == nil || *serial.Target.Port != 0 {
			t.Error("serial target port should be 0")
		}
	}

	if len(domain.Devices.Consoles) == 0 {
		t.Error("no console devices defined")
	} else {
		console := domain.Devices.Consoles[0]
		if console.Source == nil || console.Source.Pty == nil {
			t.Error("console should have pty source")
		}
		if console.Target == nil || console.Target.Type != "serial" {
			t.Error("console target type should be serial")
		}
	}

	// Validate memballoon
	if domain.Devices.MemBalloon == nil {
		t.Error("memballoon device missing")
	} else if domain.Devices.MemBalloon.Model != "virtio" {
		t.Errorf("memballoon model = %v, want virtio", domain.Devices.MemBalloon.Model)
	}

	// Validate RNG device
	if len(domain.Devices.RNGs) == 0 {
		t.Error("RNG device missing")
	} else {
		rng := domain.Devices.RNGs[0]
		if rng.Model != "virtio" {
			t.Errorf("RNG model = %v, want virtio", rng.Model)
		}
		if rng.Backend == nil || rng.Backend.Random == nil {
			t.Error("RNG backend should have random source")
		}
		if rng.Backend != nil && rng.Backend.Random != nil && rng.Backend.Random.Device != "/dev/urandom" {
			t.Error("RNG backend device should be /dev/urandom")
		}
	}
}

func TestGenerateDomainXML_XMLFormat(t *testing.T) {
	// Test that generated XML contains expected elements in proper format
	cfg := &config.VMConfig{
		Name:      "format-test",
		VCPUs:     2,
		MemoryGiB: 4,
		BootDisk: config.BootDiskConfig{
			SizeGB: 20,
			Image:  "/var/lib/libvirt/images/test.qcow2",
		},
		Network: []config.NetworkInterface{
			{
				IP:            "10.0.0.10/24",
				Gateway:       "10.0.0.1",
				Bridge:        "br0",
				MACAddress:    "be:ef:0a:00:00:0a",
				InterfaceName: "vm0a00000a",
			},
		},
	}

	xml, err := GenerateDomainXML(cfg)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}

	// Verify key XML elements are present
	requiredElements := []string{
		`<domain type="kvm"`,
		`<name>format-test</name>`,
		`<memory unit="GiB">4</memory>`,
		`<vcpu placement="static">2</vcpu>`,
		`firmware="efi"`,
		`<type arch="x86_64"`,
		`<bios useserial="yes"`,
		`<cpu mode="host-model"`,
		`<clock offset="utc"`,
		`<on_poweroff>destroy</on_poweroff>`,
		`<on_reboot>restart</on_reboot>`,
		`<on_crash>restart</on_crash>`,
		`<disk `,
		`type="qcow2"`,
		`cache="none"`,
		`dev="vda"`,
		`bus="virtio"`,
		`<boot order="1"`,
		`<interface type="bridge"`,
		`<mac address="be:ef:0a:00:00:0a"`,
		`<source bridge="br0"`,
		`<model type="virtio"`,
		`<target dev="vm0a00000a"`,
		`<serial type="pty"`,
		`<console type="pty"`,
		`<memballoon model="virtio"`,
		`<rng model="virtio"`,
	}

	for _, elem := range requiredElements {
		if !strings.Contains(xml, elem) {
			t.Errorf("Generated XML missing element: %s\n\nGenerated XML:\n%s", elem, xml)
		}
	}
}
