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

func TestGenerateDomainXML_HostDevice(t *testing.T) {
	vm := extrasTestVM(
		v1alpha1.NetworkInterfaceSpec{IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0", HostDevice: "port-a1"},
		v1alpha1.NetworkInterfaceSpec{IP: "10.20.31.40/24", Gateway: "10.20.31.1", Bridge: "br1"},
	)
	_, domain := generate(t, vm, v1alpha1.ArchX86_64)
	ifs := domain.Devices.Interfaces
	if len(ifs) != 2 {
		t.Fatalf("Interface count = %d, want 2", len(ifs))
	}
	for i, want := range []string{"port-a1", "vm0a141f28"} {
		if ifs[i].Target == nil || ifs[i].Target.Dev != want {
			t.Errorf("interface %d target = %+v, want dev %q", i, ifs[i].Target, want)
		}
	}
	// The MAC still comes from the IP whatever the device is called.
	if ifs[0].MAC == nil || ifs[0].MAC.Address != "be:ef:0a:14:1e:28" {
		t.Errorf("interface 0 MAC = %+v, want be:ef:0a:14:1e:28", ifs[0].MAC)
	}
}

func TestGenerateDomainXML_VlanTrunk(t *testing.T) {
	vm := extrasTestVM(
		v1alpha1.NetworkInterfaceSpec{IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0", DefaultRoute: true},
		v1alpha1.NetworkInterfaceSpec{IP: "10.20.31.40/24", Gateway: "10.20.31.1", Bridge: "br-trunk", VlanTrunk: []uint{101, 102, 110}},
		v1alpha1.NetworkInterfaceSpec{IP: "10.20.32.40/24", Gateway: "10.20.32.1", Bridge: "br-trunk", VlanTrunk: []uint{200}},
	)
	xml, domain := generate(t, vm, v1alpha1.ArchX86_64)
	if !strings.Contains(xml, `<vlan trunk="yes">`) {
		t.Errorf("XML has no <vlan trunk=\"yes\">:\n%s", xml)
	}
	ifs := domain.Devices.Interfaces
	if len(ifs) != 3 {
		t.Fatalf("Interface count = %d, want 3", len(ifs))
	}
	if ifs[0].VLan != nil {
		t.Errorf("Interface 0 has unexpected <vlan> element")
	}
	for i, want := range map[int][]uint{1: {101, 102, 110}, 2: {200}} {
		v := ifs[i].VLan
		if v == nil {
			t.Errorf("Interface %d missing <vlan> element", i)
			continue
		}
		if v.Trunk != "yes" {
			t.Errorf("Interface %d trunk = %q, want yes", i, v.Trunk)
		}
		if len(v.Tags) != len(want) {
			t.Errorf("Interface %d has %d tags, want %d", i, len(v.Tags), len(want))
			continue
		}
		for j, id := range want {
			if v.Tags[j].ID != id || v.Tags[j].NativeMode != "" {
				t.Errorf("Interface %d tag %d = %+v, want id %d with no native mode", i, j, v.Tags[j], id)
			}
		}
	}
}

// TestGenerateDomainXML_HostDeviceEscaped: the loader allows any printable
// ASCII the kernel takes, markup characters included; the XML must still
// carry the name unchanged.
func TestGenerateDomainXML_HostDeviceEscaped(t *testing.T) {
	const name = `a<b>&'"c`
	vm := extrasTestVM(v1alpha1.NetworkInterfaceSpec{IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0", HostDevice: name})
	_, domain := generate(t, vm, v1alpha1.ArchX86_64)
	if got := domain.Devices.Interfaces[0].Target.Dev; got != name {
		t.Errorf("target dev = %q, want %q", got, name)
	}
}

func TestGenerateDomainXML_NetworkExtrasAarch64(t *testing.T) {
	vm := extrasTestVM(v1alpha1.NetworkInterfaceSpec{
		IP: "10.20.30.40/24", Gateway: "10.20.30.1", Bridge: "br0",
		HostDevice: "port-a1", VlanTrunk: []uint{10, 20},
	})
	vm.Spec.Arch = v1alpha1.ArchAarch64
	_, domain := generate(t, vm, v1alpha1.ArchAarch64)
	iface := domain.Devices.Interfaces[0]
	if iface.Target == nil || iface.Target.Dev != "port-a1" {
		t.Errorf("target = %+v, want dev port-a1", iface.Target)
	}
	if iface.VLan == nil || iface.VLan.Trunk != "yes" || len(iface.VLan.Tags) != 2 {
		t.Errorf("vlan = %+v, want a trunk of 10 and 20", iface.VLan)
	}
}
