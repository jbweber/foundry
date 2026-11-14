package loader

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// namesTestVM is a valid spec with the given name and data disk devices.
func namesTestVM(name string, devices ...string) *v1alpha1.VirtualMachine {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: name},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk:  v1alpha1.BootDiskSpec{SizeGB: 50, Image: "fedora-43.qcow2"},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}
	for _, d := range devices {
		vm.Spec.DataDisks = append(vm.Spec.DataDisks, v1alpha1.DataDiskSpec{Device: d, SizeGB: 1})
	}
	return vm
}

// TestValidateSpec_VMName: a new VM's name is 1 to 63 characters of
// a-z 0-9 . -, starting with a letter or digit. No "_", so "<name>_" is the
// volume prefix of exactly one VM.
func TestValidateSpec_VMName(t *testing.T) {
	for _, name := range []string{"a", "web-01", "db.prod", "0vm", strings.Repeat("a", 63)} {
		if err := validateSpec(namesTestVM(name)); err != nil {
			t.Errorf("name %q refused: %v", name, err)
		}
	}
	for _, name := range []string{"a_b", "web_2", "-a", ".a", "a/b", "a b", "A", strings.Repeat("a", 64)} {
		err := validateSpec(namesTestVM(name))
		if err == nil || !strings.Contains(err.Error(), "metadata.name") {
			t.Errorf("name %q: got %v, want a metadata.name error", name, err)
		}
	}
}

// TestLoadFromYAML_VMNameLowerCased: an upper-case name is lower-cased before
// the name rule applies, as before.
func TestLoadFromYAML_VMNameLowerCased(t *testing.T) {
	vm, err := LoadFromYAML([]byte(strings.Replace(dataDeviceBase, "name: a", "name: Web-01", 1)))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if vm.Name != "web-01" {
		t.Errorf("name %q, want web-01", vm.Name)
	}
}

const dataDeviceBase = `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: a
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

// TestLoadFromYAML_DataDiskDevice: a data disk's device is a virtio disk
// name after vda. It becomes part of the volume name, so a device such as
// "vdb_boot" would name another VM's boot volume.
func TestLoadFromYAML_DataDiskDevice(t *testing.T) {
	for _, device := range []string{"vdb", "vdz", "vdbb", "vdzz"} {
		spec := dataDeviceBase + "  dataDisks:\n    - device: " + device + "\n      sizeGB: 1\n"
		if _, err := LoadFromYAML([]byte(spec)); err != nil {
			t.Errorf("device %q refused: %v", device, err)
		}
	}
	for name, device := range map[string]string{
		"takes another VM's boot volume name": "vdb_boot",
		"is the boot disk's target":           "vda",
		"has a slash":                         "x/y",
		"is not a virtio disk":                "sdb",
		"is too long":                         "vdbbb",
		"is upper case":                       "VDB",
	} {
		spec := dataDeviceBase + "  dataDisks:\n    - device: " + device + "\n      sizeGB: 1\n"
		_, err := LoadFromYAML([]byte(spec))
		if err == nil || !strings.Contains(err.Error(), "spec.dataDisks[0].device") {
			t.Errorf("device %q (%s): got %v, want a spec.dataDisks[0].device error", device, name, err)
		}
	}
}
