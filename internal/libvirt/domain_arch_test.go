package libvirt

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// archTestVM returns a VM with one bridged interface and cloud-init, so every
// arch-dependent device (cdrom bus, controller) is present.
func archTestVM(arch string) *v1alpha1.VirtualMachine {
	return &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "arch-vm"},
		Spec: v1alpha1.VirtualMachineSpec{
			Arch:      arch,
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 20,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0", DefaultRoute: true},
			},
			CloudInit: &v1alpha1.CloudInitSpec{FQDN: "arch-vm.example.com"},
		},
	}
}

func TestGenerateDomainXML_Arch(t *testing.T) {
	tests := []struct {
		name           string
		hostArch       string
		arch           string
		wantType       string
		wantArch       string
		wantMachine    string
		wantCPUMode    string
		wantCdromBus   string
		wantSCSICtrl   bool
		wantX86Feature bool // ACPI+APIC+PAE, x86 timers, bios useserial, cpu model fallback
	}{
		{
			name:           "no arch on x86_64 host",
			hostArch:       v1alpha1.ArchX86_64,
			wantType:       "kvm",
			wantArch:       "x86_64",
			wantMachine:    "q35",
			wantCPUMode:    "host-model",
			wantCdromBus:   "sata",
			wantX86Feature: true,
		},
		{
			// A spec without arch is unchanged whatever the host
			name:           "no arch on aarch64 host",
			hostArch:       v1alpha1.ArchAarch64,
			wantType:       "kvm",
			wantArch:       "x86_64",
			wantMachine:    "q35",
			wantCPUMode:    "host-model",
			wantCdromBus:   "sata",
			wantX86Feature: true,
		},
		{
			name:           "x86_64 explicit",
			hostArch:       v1alpha1.ArchX86_64,
			arch:           v1alpha1.ArchX86_64,
			wantType:       "kvm",
			wantArch:       "x86_64",
			wantMachine:    "q35",
			wantCPUMode:    "host-model",
			wantCdromBus:   "sata",
			wantX86Feature: true,
		},
		{
			name:           "x86_64 emulated on aarch64 host",
			hostArch:       v1alpha1.ArchAarch64,
			arch:           v1alpha1.ArchX86_64,
			wantType:       "qemu",
			wantArch:       "x86_64",
			wantMachine:    "q35",
			wantCPUMode:    "maximum",
			wantCdromBus:   "sata",
			wantX86Feature: true,
		},
		{
			name:         "aarch64 kvm",
			hostArch:     v1alpha1.ArchAarch64,
			arch:         v1alpha1.ArchAarch64,
			wantType:     "kvm",
			wantArch:     "aarch64",
			wantMachine:  "virt",
			wantCPUMode:  "host-passthrough",
			wantCdromBus: "scsi",
			wantSCSICtrl: true,
		},
		{
			name:         "aarch64 emulated on x86_64 host",
			hostArch:     v1alpha1.ArchX86_64,
			arch:         v1alpha1.ArchAarch64,
			wantType:     "qemu",
			wantArch:     "aarch64",
			wantMachine:  "virt",
			wantCPUMode:  "maximum",
			wantCdromBus: "scsi",
			wantSCSICtrl: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := GenerateDomainXML(archTestVM(tt.arch), tt.hostArch)
			if err != nil {
				t.Fatalf("GenerateDomainXML() error = %v", err)
			}

			var domain libvirtxml.Domain
			if err := domain.Unmarshal(xml); err != nil {
				t.Fatalf("Failed to unmarshal generated XML: %v", err)
			}

			if domain.Type != tt.wantType {
				t.Errorf("Domain type = %s, want %s", domain.Type, tt.wantType)
			}
			if domain.OS.Type.Arch != tt.wantArch {
				t.Errorf("OS arch = %s, want %s", domain.OS.Type.Arch, tt.wantArch)
			}
			if domain.OS.Type.Machine != tt.wantMachine {
				t.Errorf("OS machine = %s, want %s", domain.OS.Type.Machine, tt.wantMachine)
			}
			if domain.OS.Firmware != "efi" {
				t.Errorf("OS firmware = %s, want efi", domain.OS.Firmware)
			}
			if domain.CPU.Mode != tt.wantCPUMode {
				t.Errorf("CPU mode = %s, want %s", domain.CPU.Mode, tt.wantCPUMode)
			}

			// Features: ACPI everywhere; APIC/PAE on x86, GICv3 on Arm.
			if domain.Features.ACPI == nil {
				t.Error("ACPI feature should be enabled")
			}
			if got := domain.Features.APIC != nil && domain.Features.PAE != nil; got != tt.wantX86Feature {
				t.Errorf("APIC+PAE present = %v, want %v", got, tt.wantX86Feature)
			}
			if tt.wantX86Feature {
				if domain.Features.GIC != nil {
					t.Error("GIC feature should not be set on x86_64")
				}
			} else if domain.Features.GIC == nil || domain.Features.GIC.Version != "3" {
				t.Errorf("GIC feature = %+v, want version 3", domain.Features.GIC)
			}

			// x86-only settings
			if got := len(domain.Clock.Timer) > 0; got != tt.wantX86Feature {
				t.Errorf("Clock timers present = %v, want %v", got, tt.wantX86Feature)
			}
			if got := domain.OS.BIOS != nil; got != tt.wantX86Feature {
				t.Errorf("<bios useserial> present = %v, want %v", got, tt.wantX86Feature)
			}
			if got := domain.CPU.Model != nil; got != tt.wantX86Feature {
				t.Errorf("CPU model fallback present = %v, want %v", got, tt.wantX86Feature)
			}

			// Cloud-init cdrom bus and its controller
			var cdrom *libvirtxml.DomainDisk
			for i := range domain.Devices.Disks {
				if domain.Devices.Disks[i].Device == "cdrom" {
					cdrom = &domain.Devices.Disks[i]
				}
			}
			if cdrom == nil {
				t.Fatal("cloud-init cdrom not found")
			}
			if cdrom.Target.Bus != tt.wantCdromBus || cdrom.Target.Dev != "sda" {
				t.Errorf("cdrom target = %s/%s, want sda/%s", cdrom.Target.Dev, cdrom.Target.Bus, tt.wantCdromBus)
			}

			hasSCSICtrl := false
			for _, ctrl := range domain.Devices.Controllers {
				if ctrl.Type == "scsi" && ctrl.Model == "virtio-scsi" {
					hasSCSICtrl = true
				}
			}
			if hasSCSICtrl != tt.wantSCSICtrl {
				t.Errorf("virtio-scsi controller present = %v, want %v", hasSCSICtrl, tt.wantSCSICtrl)
			}

			// Serial console stays on port 0 with a pty on every arch.
			if len(domain.Devices.Serials) != 1 || len(domain.Devices.Consoles) != 1 {
				t.Errorf("serial/console count = %d/%d, want 1/1", len(domain.Devices.Serials), len(domain.Devices.Consoles))
			}
		})
	}
}

func TestGenerateDomainXML_NoArchUnchanged(t *testing.T) {
	// A spec without arch renders the same XML whatever the host, and the
	// same as an explicit x86_64 on an x86_64 host.
	want, err := GenerateDomainXML(archTestVM(""), v1alpha1.ArchX86_64)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}
	for _, hostArch := range []string{v1alpha1.ArchAarch64, ""} {
		got, err := GenerateDomainXML(archTestVM(""), hostArch)
		if err != nil {
			t.Fatalf("GenerateDomainXML(host %q) error = %v", hostArch, err)
		}
		if got != want {
			t.Errorf("XML on host %q differs from x86_64 host:\n%s\nwant:\n%s", hostArch, got, want)
		}
	}
	explicit, err := GenerateDomainXML(archTestVM(v1alpha1.ArchX86_64), v1alpha1.ArchX86_64)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}
	if explicit != want {
		t.Errorf("explicit x86_64 XML differs from no arch:\n%s\nwant:\n%s", explicit, want)
	}
}

func TestGenerateDomainXML_Aarch64NoCloudInit(t *testing.T) {
	// Without cloud-init there is no cdrom, so no SCSI controller either.
	vm := archTestVM(v1alpha1.ArchAarch64)
	vm.Spec.CloudInit = nil

	xml, err := GenerateDomainXML(vm, v1alpha1.ArchAarch64)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}
	if strings.Contains(xml, `type="scsi"`) {
		t.Errorf("Unexpected SCSI controller without cloud-init:\n%s", xml)
	}
}

func TestGenerateDomainXML_EmulatedHostCPUMode(t *testing.T) {
	for _, mode := range []string{v1alpha1.CPUModeHostModel, v1alpha1.CPUModeHostPassthrough} {
		t.Run(mode, func(t *testing.T) {
			vm := archTestVM(v1alpha1.ArchX86_64)
			vm.Spec.CPUMode = mode

			_, err := GenerateDomainXML(vm, v1alpha1.ArchAarch64)
			if err == nil || !strings.Contains(err.Error(), "emulated x86_64 guest") {
				t.Fatalf("GenerateDomainXML() error = %v, want an emulated-guest error", err)
			}
		})
	}
}
