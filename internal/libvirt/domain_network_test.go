package libvirt

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// extrasTestVM is a minimal valid VM for the disk and interface extras.
func extrasTestVM(ifaces ...v1alpha1.NetworkInterfaceSpec) *v1alpha1.VirtualMachine {
	if len(ifaces) == 0 {
		ifaces = []v1alpha1.NetworkInterfaceSpec{{IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0"}}
	}
	return &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "extras-vm"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:             2,
			MemoryGiB:         4,
			BootDisk:          v1alpha1.BootDiskSpec{SizeGB: 20, Image: "fedora-43.qcow2"},
			NetworkInterfaces: ifaces,
		},
	}
}

func generate(t *testing.T, vm *v1alpha1.VirtualMachine, arch string) (string, *libvirtxml.Domain) {
	t.Helper()
	xml, err := GenerateDomainXML(vm, arch)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}
	var domain libvirtxml.Domain
	if err := domain.Unmarshal(xml); err != nil {
		t.Fatalf("Failed to unmarshal generated XML: %v", err)
	}
	return xml, &domain
}

func TestGenerateDomainXML_DiskSerial(t *testing.T) {
	for _, arch := range []string{v1alpha1.ArchX86_64, v1alpha1.ArchAarch64} {
		t.Run(arch, func(t *testing.T) {
			vm := extrasTestVM()
			vm.Spec.Arch = arch
			vm.Spec.BootDisk.Serial = "root-0"
			vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{
				{Device: "vdb", SizeGB: 10, Serial: "data-0"},
				{Device: "vdc", SizeGB: 10}, // no serial: no element
			}
			vm.Spec.CloudInit = &v1alpha1.CloudInitSpec{FQDN: "extras-vm.example.com"}

			xml, domain := generate(t, vm, arch)
			if !strings.Contains(xml, "<serial>data-0</serial>") {
				t.Errorf("XML has no <serial>data-0</serial>:\n%s", xml)
			}
			want := map[string]string{"vda": "root-0", "vdb": "data-0", "vdc": ""}
			for _, disk := range domain.Devices.Disks {
				if disk.Target == nil {
					continue
				}
				if disk.Device != "disk" {
					// The cloud-init seed and input disks are CD-ROMs and carry no serial.
					if disk.Serial != "" {
						t.Errorf("CD-ROM %s has serial %q", disk.Target.Dev, disk.Serial)
					}
					continue
				}
				w, ok := want[disk.Target.Dev]
				if !ok {
					t.Errorf("unexpected disk %s", disk.Target.Dev)
					continue
				}
				if disk.Serial != w {
					t.Errorf("disk %s serial = %q, want %q", disk.Target.Dev, disk.Serial, w)
				}
				delete(want, disk.Target.Dev)
			}
			if len(want) != 0 {
				t.Errorf("disks missing from XML: %v", want)
			}
		})
	}
}

func TestGenerateDomainXML_NoDiskSerialByDefault(t *testing.T) {
	xml, _ := generate(t, extrasTestVM(), v1alpha1.ArchX86_64)
	// The console <serial type='pty'> stays; a disk <serial> element must not appear.
	if strings.Contains(xml, "<serial>") {
		t.Errorf("XML has a disk <serial> element without one in the spec:\n%s", xml)
	}
}
