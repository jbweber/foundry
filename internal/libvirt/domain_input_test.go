package libvirt

import (
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// Input disks are read-only CD-ROMs on their pool volumes, after the cloud-init
// ISO (sdb, sdc, ...), on SATA (x86_64) or virtio-scsi (aarch64), with one SCSI
// controller for all CD-ROMs.
func TestGenerateDomainXML_InputDisks(t *testing.T) {
	inputs := []v1alpha1.InputDiskSpec{
		{Label: "NODECFG", Files: []v1alpha1.InputFileSpec{{Name: "node.yaml", String: "x"}}},
		{Label: "EXTRA_2", Files: []v1alpha1.InputFileSpec{{Name: "a", String: "a"}}},
	}
	for _, tc := range []struct {
		arch, bus       string
		scsiControllers int
	}{
		{v1alpha1.ArchX86_64, "sata", 0},
		{v1alpha1.ArchAarch64, "scsi", 1},
	} {
		vm := archTestVM(tc.arch)
		vm.Spec.InputDisks = inputs
		d := renderDomain(t, vm, tc.arch)
		type disk struct{ dev, bus, vol string }
		var cdroms []disk
		for _, dd := range d.Devices.Disks {
			if dd.Device != "cdrom" {
				continue
			}
			if dd.ReadOnly == nil || dd.Driver.Type != "raw" || dd.Source.Volume.Pool != "foundry-vms" {
				t.Errorf("%s: cdrom %s not a read-only raw volume of foundry-vms", tc.arch, dd.Target.Dev)
			}
			cdroms = append(cdroms, disk{dd.Target.Dev, dd.Target.Bus, dd.Source.Volume.Volume})
		}
		want := []disk{
			{"sda", tc.bus, "arch-vm_cloudinit.iso"},
			{"sdb", tc.bus, "arch-vm_input-nodecfg.iso"},
			{"sdc", tc.bus, "arch-vm_input-extra_2.iso"},
		}
		if len(cdroms) != len(want) {
			t.Fatalf("%s: cdroms %v", tc.arch, cdroms)
		}
		for i := range want {
			if cdroms[i] != want[i] {
				t.Errorf("%s: cdrom %d = %v, want %v", tc.arch, i, cdroms[i], want[i])
			}
		}
		n := 0
		for _, c := range d.Devices.Controllers {
			if c.Type == "scsi" {
				n++
			}
		}
		if n != tc.scsiControllers {
			t.Errorf("%s: %d SCSI controllers, want %d", tc.arch, n, tc.scsiControllers)
		}
	}

	// Without cloud-init, the input disks keep their targets, and aarch64
	// still gets its SCSI controller
	vm := archTestVM(v1alpha1.ArchAarch64)
	vm.Spec.CloudInit = nil
	vm.Spec.InputDisks = inputs[:1]
	d := renderDomain(t, vm, v1alpha1.ArchAarch64)
	last := d.Devices.Disks[len(d.Devices.Disks)-1]
	if last.Target.Dev != "sdb" || last.Target.Bus != "scsi" {
		t.Errorf("input disk without cloud-init: %s on %s", last.Target.Dev, last.Target.Bus)
	}
	hasSCSI := false
	for _, c := range d.Devices.Controllers {
		hasSCSI = hasSCSI || c.Type == "scsi"
	}
	if !hasSCSI {
		t.Error("aarch64 input disk without a SCSI controller")
	}
}
