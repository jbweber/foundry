package libvirt

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestGenerateDomainXML(t *testing.T) {
	tests := []struct {
		name    string
		vm      *v1alpha1.VirtualMachine
		wantErr bool
	}{
		{
			name: "simple VM with cloud-init",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "test-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     4,
					MemoryGiB: 8,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "/var/lib/libvirt/images/fedora-42.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.20.30.40/24",
							Gateway:      "10.20.30.1",
							DNSServers:   []string{"8.8.8.8", "1.1.1.1"},
							Bridge:       "br0",
							DefaultRoute: true,
						},
					},
					CloudInit: &v1alpha1.CloudInitSpec{
						FQDN:              "test-vm.example.com",
						SSHAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo test@example.com"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with multiple data disks",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "multi-disk-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 30,
						Image:  "/var/lib/libvirt/images/ubuntu-24.04.qcow2",
					},
					DataDisks: []v1alpha1.DataDiskSpec{
						{Device: "vdb", SizeGB: 100},
						{Device: "vdc", SizeGB: 200},
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "192.168.1.50/24",
							Gateway:      "192.168.1.1",
							Bridge:       "br1",
							DefaultRoute: true,
						},
					},
					CloudInit: &v1alpha1.CloudInitSpec{
						FQDN: "multi-disk.local",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM without cloud-init",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "no-cloudinit-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     8,
					MemoryGiB: 16,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 100,
						Empty:  true,
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.55.22.22/24",
							Gateway:      "10.55.22.1",
							Bridge:       "br0",
							DefaultRoute: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with multiple NICs",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "multi-nic-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     4,
					MemoryGiB: 8,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "/var/lib/libvirt/images/base.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:           "10.20.30.40/24",
							Gateway:      "10.20.30.1",
							Bridge:       "br0",
							DefaultRoute: true,
						},
						{
							IP:      "192.168.1.100/24",
							Gateway: "192.168.1.1",
							Bridge:  "br1",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize config to set default storage pools
			tt.vm.Normalize()

			xml, err := GenerateDomainXML(tt.vm)
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
			validateDomainStructure(t, &domain, tt.vm)
		})
	}
}

// validateDomainStructure validates the domain XML structure matches Ansible reference
func validateDomainStructure(t *testing.T, domain *libvirtxml.Domain, vm *v1alpha1.VirtualMachine) {
	t.Helper()

	// Validate basic metadata
	if domain.Type != "kvm" {
		t.Errorf("domain type = %v, want kvm", domain.Type)
	}
	if domain.Name != vm.Name {
		t.Errorf("domain name = %v, want %v", domain.Name, vm.Name)
	}

	// Validate memory
	if domain.Memory == nil {
		t.Error("domain memory is nil")
	} else {
		if domain.Memory.Value != uint(vm.Spec.MemoryGiB) {
			t.Errorf("memory value = %v, want %v", domain.Memory.Value, vm.Spec.MemoryGiB)
		}
		if domain.Memory.Unit != "GiB" {
			t.Errorf("memory unit = %v, want GiB", domain.Memory.Unit)
		}
	}

	// Validate VCPUs
	if domain.VCPU == nil {
		t.Error("domain VCPU is nil")
	} else {
		if domain.VCPU.Value != uint(vm.Spec.VCPUs) {
			t.Errorf("vcpu value = %v, want %v", domain.VCPU.Value, vm.Spec.VCPUs)
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
	expectedDiskCount := 1 + len(vm.Spec.DataDisks) // boot disk + data disks
	if vm.Spec.CloudInit != nil {
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
			expectedPool := vm.GetStoragePool()
			expectedVolume := vm.GetBootVolumeName()
			if bootDisk.Source.Volume.Pool != expectedPool {
				t.Errorf("boot disk pool = %v, want %v", bootDisk.Source.Volume.Pool, expectedPool)
			}
			if bootDisk.Source.Volume.Volume != expectedVolume {
				t.Errorf("boot disk volume = %v, want %v", bootDisk.Source.Volume.Volume, expectedVolume)
			}
		}
	}

	// Validate data disks
	for i, dataDiskCfg := range vm.Spec.DataDisks {
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
			expectedPool := vm.GetStoragePool()
			expectedVolume := vm.GetDataVolumeName(dataDiskCfg.Device)
			if disk.Source.Volume.Pool != expectedPool {
				t.Errorf("data disk pool = %v, want %v", disk.Source.Volume.Pool, expectedPool)
			}
			if disk.Source.Volume.Volume != expectedVolume {
				t.Errorf("data disk volume = %v, want %v", disk.Source.Volume.Volume, expectedVolume)
			}
		}
	}

	// Validate cloud-init ISO (if configured)
	if vm.Spec.CloudInit != nil {
		cdromIdx := 1 + len(vm.Spec.DataDisks)
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
				expectedPool := vm.GetStoragePool()
				expectedVolume := vm.GetCloudInitVolumeName()
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
	if len(domain.Devices.Interfaces) != len(vm.Spec.NetworkInterfaces) {
		t.Errorf("interface count = %v, want %v", len(domain.Devices.Interfaces), len(vm.Spec.NetworkInterfaces))
	}
	for i, ifaceCfg := range vm.Spec.NetworkInterfaces {
		if len(domain.Devices.Interfaces) <= i {
			t.Errorf("network interface %v missing", i)
			continue
		}
		iface := domain.Devices.Interfaces[i]
		// Note: interface type is determined by which Source union member is set
		if iface.Source == nil || iface.Source.Bridge == nil {
			t.Errorf("interface %v should have bridge source", i)
		}
		// MAC address is calculated from IP, so we verify it exists but don't check the exact value
		if iface.MAC == nil || iface.MAC.Address == "" {
			t.Errorf("interface %v MAC address is missing", i)
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
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "format-test",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 20,
				Image:  "/var/lib/libvirt/images/test.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:           "10.0.0.10/24",
					Gateway:      "10.0.0.1",
					Bridge:       "br0",
					DefaultRoute: true,
				},
			},
		},
	}

	xml, err := GenerateDomainXML(vm)
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
		`<mac address=`,
		`<source bridge="br0"`,
		`<model type="virtio"`,
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

// TestGenerateDomainXML_EdgeCases tests edge cases for domain XML generation
func TestGenerateDomainXML_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		vm      *v1alpha1.VirtualMachine
		wantErr bool
		errMsg  string
	}{
		{
			name: "VM with custom CPU mode",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "custom-cpu-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     4,
					MemoryGiB: 8,
					CPUMode:   "host-passthrough",
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with minimal memory (512MB)",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "minimal-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     1,
					MemoryGiB: 1, // Minimal memory
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 10,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with large memory (256GB)",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "large-memory-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     32,
					MemoryGiB: 256, // Large memory
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 500,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with many VCPUs (32)",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "many-vcpu-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     32,
					MemoryGiB: 64,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 100,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with long name",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "this-is-a-very-long-vm-name-that-tests-name-length-handling",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with name containing hyphens",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "web-server-prod-01",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with many data disks (4 disks)",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "many-disk-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     4,
					MemoryGiB: 8,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 50,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					DataDisks: []v1alpha1.DataDiskSpec{
						{Device: "vdb", SizeGB: 100},
						{Device: "vdc", SizeGB: 200},
						{Device: "vdd", SizeGB: 300},
						{Device: "vde", SizeGB: 400},
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with multiple NICs on different bridges",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "multi-bridge-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
						{
							IP:      "192.168.1.50/24",
							Gateway: "192.168.1.1",
							Bridge:  "br1",
						},
						{
							IP:      "172.16.0.100/16",
							Gateway: "172.16.0.1",
							Bridge:  "br2",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM with invalid IP in network interface",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "invalid-ip-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "not-an-ip-address",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "failed to calculate MAC address",
		},
		{
			name: "VM with IPv6 address (unsupported)",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "ipv6-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "2001:db8::1/64",
							Gateway: "2001:db8::ffff",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "only IPv4 addresses are supported",
		},
		{
			name: "VM with custom storage pool",
			vm: &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{
					Name: "custom-pool-vm",
				},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:       2,
					MemoryGiB:   4,
					StoragePool: "custom-pool",
					BootDisk: v1alpha1.BootDiskSpec{
						SizeGB: 20,
						Image:  "/var/lib/libvirt/images/test.qcow2",
					},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{
							IP:      "10.0.0.10/24",
							Gateway: "10.0.0.1",
							Bridge:  "br0",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize config to set defaults
			tt.vm.Normalize()

			xml, err := GenerateDomainXML(tt.vm)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateDomainXML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("GenerateDomainXML() error = %v, want error containing %q", err, tt.errMsg)
				}
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
				t.Errorf("Generated XML cannot be unmarshaled: %v", err)
				return
			}

			// Verify CPU mode if specified
			if tt.vm.Spec.CPUMode != "" {
				if domain.CPU == nil || domain.CPU.Mode != tt.vm.Spec.CPUMode {
					t.Errorf("CPU mode = %v, want %v", domain.CPU.Mode, tt.vm.Spec.CPUMode)
				}
			}

			// Verify custom storage pool if specified
			if tt.vm.Spec.StoragePool != "" {
				if len(domain.Devices.Disks) > 0 {
					bootDisk := domain.Devices.Disks[0]
					if bootDisk.Source != nil && bootDisk.Source.Volume != nil {
						if bootDisk.Source.Volume.Pool != tt.vm.Spec.StoragePool {
							t.Errorf("Boot disk pool = %v, want %v", bootDisk.Source.Volume.Pool, tt.vm.Spec.StoragePool)
						}
					}
				}
			}
		})
	}
}

// TestCalculateMACFromIP_EdgeCases tests edge cases for MAC calculation
func TestCalculateMACFromIP_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name:    "IP with CIDR",
			ip:      "10.20.30.40/24",
			want:    "be:ef:0a:14:1e:28",
			wantErr: false,
		},
		{
			name:    "IP without CIDR",
			ip:      "192.168.1.100",
			want:    "be:ef:c0:a8:01:64",
			wantErr: false,
		},
		{
			name:    "invalid IP",
			ip:      "not-an-ip",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			ip:      "10.20.30.40/99",
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 address",
			ip:      "2001:db8::1",
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 with CIDR",
			ip:      "fe80::1/64",
			want:    "",
			wantErr: true,
		},
		{
			name:    "edge IP 0.0.0.0",
			ip:      "0.0.0.0/0",
			want:    "be:ef:00:00:00:00",
			wantErr: false,
		},
		{
			name:    "edge IP 255.255.255.255",
			ip:      "255.255.255.255/32",
			want:    "be:ef:ff:ff:ff:ff",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateMACFromIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateMACFromIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("calculateMACFromIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCalculateInterfaceNameFromIP_EdgeCases tests edge cases for interface name calculation
func TestCalculateInterfaceNameFromIP_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name:    "IP with CIDR",
			ip:      "10.20.30.40/24",
			want:    "vm0a141e28",
			wantErr: false,
		},
		{
			name:    "IP without CIDR",
			ip:      "192.168.1.100",
			want:    "vmc0a80164",
			wantErr: false,
		},
		{
			name:    "invalid IP",
			ip:      "invalid",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			ip:      "10.20.30.40/999",
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 address",
			ip:      "2001:db8::1",
			want:    "",
			wantErr: true,
		},
		{
			name:    "IPv6 with CIDR",
			ip:      "fe80::1/64",
			want:    "",
			wantErr: true,
		},
		{
			name:    "edge IP 0.0.0.0",
			ip:      "0.0.0.0/0",
			want:    "vm00000000",
			wantErr: false,
		},
		{
			name:    "edge IP 255.255.255.255",
			ip:      "255.255.255.255/32",
			want:    "vmffffffff",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateInterfaceNameFromIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateInterfaceNameFromIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("calculateInterfaceNameFromIP() = %v, want %v", got, tt.want)
			}
		})
	}
}
