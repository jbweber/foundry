package loader

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// Secure Boot on aarch64 needs firmware built for the uefi-vars device: a flash
// build under <varstore> would boot with Secure Boot off. x86_64 is not checked
// for this, whatever the name.
func TestValidateFirmware_SecureBootByArch(t *testing.T) {
	const qemuVars = "/usr/share/edk2/aarch64/QEMU_EFI.qemuvars.fd"
	tests := []struct {
		name    string
		arch    string
		fw      v1alpha1.FirmwareSpec
		wantErr string
	}{
		{"aarch64, firmware's own keys", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: qemuVars, SecureBoot: true}, ""},
		{"aarch64, key profile", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: qemuVars, SecureBoot: true, Keys: "corp-keys"}, ""},
		{"aarch64, JSON template", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: qemuVars, SecureBoot: true, NVRAMTemplate: "/srv/keys.json"}, ""},
		{"aarch64, flash template", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: qemuVars, SecureBoot: true, NVRAMTemplate: "/srv/VARS.fd"}, "uefi-vars JSON store"},
		{"aarch64, AAVMF flash build", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: "/usr/share/AAVMF/AAVMF_CODE.fd", SecureBoot: true}, "uefi-vars device"},
		{"aarch64, plain ROM build", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI.fd", SecureBoot: true}, "uefi-vars device"},
		{"aarch64, the flash form of the right build", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI-qemuvars-pflash.raw", SecureBoot: true}, "uefi-vars device"},
		{"aarch64, both keys and template", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: qemuVars, SecureBoot: true, Keys: "k", NVRAMTemplate: "/srv/keys.json"}, "mutually exclusive"},
		{"aarch64 without Secure Boot, any firmware", v1alpha1.ArchAarch64, v1alpha1.FirmwareSpec{Loader: "/usr/share/AAVMF/AAVMF_CODE.fd", NVRAMTemplate: "/srv/VARS.fd"}, ""},
		{"x86_64 Secure Boot", v1alpha1.ArchX86_64, v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd", SecureBoot: true, NVRAMTemplate: "/srv/VARS.fd"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := tt.fw
			err := validateFirmware(&v1alpha1.VirtualMachine{Spec: v1alpha1.VirtualMachineSpec{Arch: tt.arch, Firmware: &fw}})
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateFirmware() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateFirmware() = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
