package loader

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestValidateSpec_Firmware(t *testing.T) {
	const loader = "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd"
	tests := []struct {
		name    string
		arch    string
		fw      *v1alpha1.FirmwareSpec
		wantErr string
	}{
		{name: "no firmware"},
		{name: "loader only", fw: &v1alpha1.FirmwareSpec{Loader: loader}},
		{name: "secure boot with template", fw: &v1alpha1.FirmwareSpec{Loader: loader, SecureBoot: true, NVRAMTemplate: "/srv/VARS.fd"}},
		{name: "secure boot with keys", fw: &v1alpha1.FirmwareSpec{Loader: loader, SecureBoot: true, Keys: "my-keys"}},
		{name: "aarch64 keys without secure boot", arch: v1alpha1.ArchAarch64, fw: &v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI-silent-pflash.raw", Keys: "k1"}},
		{name: "missing loader", fw: &v1alpha1.FirmwareSpec{SecureBoot: true}, wantErr: "spec.firmware.loader is required"},
		{name: "aarch64 secure boot", arch: v1alpha1.ArchAarch64, fw: &v1alpha1.FirmwareSpec{Loader: loader, SecureBoot: true}, wantErr: "only supported for x86_64"},
		{name: "keys and template", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: "k", NVRAMTemplate: "/srv/VARS.fd"}, wantErr: "mutually exclusive"},
		{name: "keys with underscore", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: "my_keys"}, wantErr: "spec.firmware.keys"},
		{name: "keys with slash", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: "../keys"}, wantErr: "spec.firmware.keys"},
		{name: "keys uppercase", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: "Keys"}, wantErr: "spec.firmware.keys"},
		{name: "keys trailing dash", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: "keys-"}, wantErr: "spec.firmware.keys"},
		{name: "keys too long", fw: &v1alpha1.FirmwareSpec{Loader: loader, Keys: strings.Repeat("k", 64)}, wantErr: "spec.firmware.keys"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{Name: "fw-vm"},
				Spec: v1alpha1.VirtualMachineSpec{
					Arch:      tt.arch,
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk:  v1alpha1.BootDiskSpec{SizeGB: 20, Image: "fedora-43.qcow2"},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
					},
					Firmware: tt.fw,
				},
			}
			err := validateSpec(vm)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSpec() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateSpec() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidKeyProfileName(t *testing.T) {
	for name, want := range map[string]bool{
		"a": true, "my-keys": true, "k8s-2026": true, strings.Repeat("a", 63): true,
		"": false, "-a": false, "a-": false, "A": false, "a_b": false, "a.b": false, strings.Repeat("a", 64): false,
	} {
		if got := ValidKeyProfileName(name); got != want {
			t.Errorf("ValidKeyProfileName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLoadFromYAML_Firmware(t *testing.T) {
	yaml := `apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: fw-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 20
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
  firmware:
    loader: /usr/share/edk2/ovmf/OVMF_CODE.secboot.fd
    secureBoot: true
    keys: my-keys
`
	vm, err := LoadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	fw := vm.Spec.Firmware
	if fw == nil || fw.Loader != "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd" || !fw.SecureBoot || fw.Keys != "my-keys" {
		t.Errorf("firmware = %+v", fw)
	}
}
