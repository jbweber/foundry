package loader

import (
	"fmt"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func nicYAML(nics string) string {
	return fmt.Sprintf(`
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: nic-vm
spec:
  vcpus: 1
  memoryGiB: 1
  bootDisk:
    sizeGB: 10
    image: base.qcow2
  networkInterfaces:
%s`, nics)
}

func TestLoadFromYAML_HostDevice(t *testing.T) {
	tests := []struct {
		name    string
		nics    string
		wantErr string
	}{
		{"set", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: port-a1}\n", ""},
		{"fifteen characters", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: abcdefghijklmno}\n", ""},
		{"default derived", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0}\n", ""},
		{"too long", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: abcdefghijklmnop}\n", "1 to 15"},
		{"vnet", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: vnet3}\n", `prefix libvirt reserves`},
		{"vif", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: vif1.0}\n", `"vif"`},
		{"macvtap", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: macvtap0}\n", `"macvtap"`},
		{"macvlan", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: macvlan0}\n", `"macvlan"`},
		{"slash", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: a/b}\n", "may not contain"},
		{"colon", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: 'a:b'}\n", "may not contain"},
		{"space", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: 'a b'}\n", "may not contain"},
		{"tab", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: \"a\\tb\"}\n", "may not contain"},
		{"non-ascii", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: pört}\n", "may not contain"},
		{"percent", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: 'port%d'}\n", "may not contain"},
		{"dot", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: '.'}\n", "cannot be"},
		{"dotdot", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: '..'}\n", "cannot be"},
		{"duplicate", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: p1}\n    - {ip: 10.0.1.2/24, gateway: 10.0.1.1, bridge: br1, hostDevice: p1}\n", "host device name"},
		// 10.0.0.3 derives vm0a000003
		{"collides with derived", "    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, hostDevice: vm0a000003}\n    - {ip: 10.0.0.3/24, gateway: 10.0.0.1, bridge: br1}\n", "host device name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromYAML([]byte(nicYAML(tt.nics)))
			checkErr(t, err, tt.wantErr)
		})
	}
}

func TestValidateSpec_VlanTrunk(t *testing.T) {
	ids := func(n int) []uint {
		out := make([]uint, n)
		for i := range out {
			out[i] = uint(i + 1)
		}
		return out
	}
	tests := []struct {
		name    string
		vlan    *uint
		trunk   []uint
		wantErr string // empty: valid
	}{
		{"absent", nil, nil, ""},
		{"one ID", nil, []uint{101}, ""},
		{"ten IDs", nil, []uint{101, 102, 103, 104, 105, 106, 107, 108, 109, 110}, ""},
		{"boundaries", nil, []uint{1, MaxVlanID}, ""},
		{"at the limit", nil, ids(MaxVlanTrunk), ""},
		{"beside vlan", vlanPtr(100), []uint{101}, "cannot set both vlan and vlanTrunk"},
		{"empty list", nil, []uint{}, "vlanTrunk is empty"},
		{"zero", nil, []uint{0}, "must be between 1 and 4094"},
		{"4095", nil, []uint{101, 4095}, "must be between 1 and 4094"},
		{"duplicate", nil, []uint{101, 102, 101}, "lists 101 twice"},
		{"over the limit", nil, ids(MaxVlanTrunk + 1), "at most 256"},
		{"with pxeBoot", nil, []uint{101}, "cannot set both pxeBoot and vlanTrunk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithInterfaces(v1alpha1.NetworkInterfaceSpec{
				IP:        "10.0.0.1/24",
				Gateway:   "10.0.0.254",
				Bridge:    "br0",
				Vlan:      tt.vlan,
				VlanTrunk: tt.trunk,
				PXEBoot:   tt.name == "with pxeBoot",
			})
			checkErr(t, validateSpec(vm), tt.wantErr)
		})
	}
}

func TestLoadFromYAML_VlanTrunk(t *testing.T) {
	vm, err := LoadFromYAML([]byte(nicYAML("    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, vlanTrunk: [101, 102, 110]}\n")))
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	got := vm.Spec.NetworkInterfaces[0].VlanTrunk
	if len(got) != 3 || got[0] != 101 || got[2] != 110 {
		t.Errorf("vlanTrunk = %v, want [101 102 110]", got)
	}
	// An empty list in YAML is a mistake, not "no trunk".
	if _, err := LoadFromYAML([]byte(nicYAML("    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, vlanTrunk: []}\n"))); err == nil {
		t.Error("vlanTrunk: [] loaded without an error")
	}
}
