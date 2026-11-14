package loader

import (
	"fmt"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestValidateSpec_DiskSerial(t *testing.T) {
	tests := []struct {
		name    string
		boot    string
		data    []string
		wantErr string // substring; empty means valid
	}{
		{name: "no serials", data: []string{"", ""}},
		{name: "data serials", data: []string{"data-01", "logs_2.a"}},
		{name: "boot and data serials", boot: "root", data: []string{"data"}},
		{name: "twenty characters", data: []string{"ABCDEFGHIJ0123456789"}},
		{name: "twenty-one characters", data: []string{"ABCDEFGHIJ0123456789x"}, wantErr: "longer than 20"},
		{name: "space", data: []string{"data 1"}, wantErr: "may only contain"},
		{name: "slash", data: []string{"data/1"}, wantErr: "may only contain"},
		{name: "markup", data: []string{"a<b>&c"}, wantErr: "may only contain"},
		{name: "non-ascii", data: []string{"dätä"}, wantErr: "may only contain"},
		{name: "duplicate data serials", data: []string{"same", "same"}, wantErr: "already used by spec.dataDisks[0].serial"},
		{name: "data serial same as boot", boot: "same", data: []string{"same"}, wantErr: "already used by spec.bootDisk.serial"},
		{name: "bad boot serial", boot: "a:b", wantErr: "spec.bootDisk.serial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithInterfaces(v1alpha1.NetworkInterfaceSpec{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"})
			vm.Spec.BootDisk.Serial = tt.boot
			for i, serial := range tt.data {
				vm.Spec.DataDisks = append(vm.Spec.DataDisks, v1alpha1.DataDiskSpec{
					Device: fmt.Sprintf("vd%c", 'b'+i),
					SizeGB: 10,
					Serial: serial,
				})
			}
			checkErr(t, validateSpec(vm), tt.wantErr)
		})
	}
}

func TestLoadFromYAML_DiskSerial(t *testing.T) {
	vm, err := LoadFromYAML([]byte(`apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: serial-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 20
    image: fedora-43.qcow2
    serial: root-0
  dataDisks:
    - device: vdb
      sizeGB: 10
      serial: data-0
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`))
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	if got := vm.Spec.BootDisk.Serial; got != "root-0" {
		t.Errorf("bootDisk.serial = %q, want root-0", got)
	}
	if got := vm.Spec.DataDisks[0].Serial; got != "data-0" {
		t.Errorf("dataDisks[0].serial = %q, want data-0", got)
	}
}
