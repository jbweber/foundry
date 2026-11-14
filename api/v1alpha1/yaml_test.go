package v1alpha1

import (
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Every string reads back unchanged, map keys included, among them those
// that yaml.Marshal writes as a block it then refuses to read, and so does
// the rest of the spec; the spec encoded is left unchanged.
func TestEncodeYAML_RoundTrip(t *testing.T) {
	for _, s := range []string{
		"\tb\nc",
		"\t\n0",
		" b\nc",
		"\tsingle line",
		"a\n\tb",
		"#!/bin/sh\ncat <<-E\n\tx\nE\n",
		"plain",
		"\xbe\n\tx",
		"\n#",
		"\n\n x",
	} {
		autostart := false
		vlan := uint(30)
		vm := &VirtualMachine{
			TypeMeta:   TypeMeta{APIVersion: GroupName + "/" + Version, Kind: VirtualMachineKind},
			ObjectMeta: ObjectMeta{Name: "a", Labels: map[string]string{"k": "true"}, Annotations: map[string]string{"\tkey\n": s}},
			Spec: VirtualMachineSpec{
				VCPUs:     2,
				MemoryGiB: 1,
				Autostart: &autostart,
				BootDisk:  BootDiskSpec{SizeGB: 1, Empty: true},
				NetworkInterfaces: []NetworkInterfaceSpec{
					{IP: "10.0.0.2/24", Bridge: "br0", VlanTrunk: []uint{10, 20}},
					{IP: "10.0.1.2/24", Bridge: "br0", Vlan: &vlan},
				},
				CloudInit:  &CloudInitSpec{RawUserData: s},
				FwCfg:      []FwCfgEntrySpec{{Name: "opt/x", String: s}},
				InputDisks: []InputDiskSpec{{Label: "CFG", Files: []InputFileSpec{{Name: "a", String: s}}}},
			},
		}
		out, err := EncodeYAML(vm)
		if err != nil {
			t.Fatalf("EncodeYAML(%q): %v", s, err)
		}
		if vm.Spec.CloudInit.RawUserData != s || vm.Annotations["\tkey\n"] != s || vm.Spec.FwCfg[0].String != s {
			t.Errorf("EncodeYAML(%q) changed the spec", s)
		}
		var got VirtualMachine
		if err := yaml.Unmarshal(out, &got); err != nil {
			t.Fatalf("%q does not read back: %v\n%s", s, err, out)
		}
		if !reflect.DeepEqual(&got, vm) {
			t.Errorf("%q: the spec reads back as\n%+v\nwant\n%+v\nYAML:\n%s", s, got, *vm, out)
		}
	}
}

// A spec that yaml.Marshal writes readably is written as yaml.Marshal writes it.
func TestEncodeYAML_SameAsMarshal(t *testing.T) {
	vm := &VirtualMachine{
		TypeMeta:   TypeMeta{APIVersion: GroupName + "/" + Version, Kind: VirtualMachineKind},
		ObjectMeta: ObjectMeta{Name: "a", Labels: map[string]string{"k": "v"}},
		Spec:       VirtualMachineSpec{VCPUs: 1, CloudInit: &CloudInitSpec{RawUserData: "#!/bin/sh\n\techo hi\n"}},
	}
	want, err := yaml.Marshal(vm)
	if err != nil {
		t.Fatal(err)
	}
	got, err := EncodeYAML(vm)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("EncodeYAML:\n%s\nyaml.Marshal:\n%s", got, want)
	}
}
