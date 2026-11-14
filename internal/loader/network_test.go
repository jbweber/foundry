package loader

import (
	"fmt"
	"testing"
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
