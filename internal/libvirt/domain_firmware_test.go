package libvirt

import (
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

func renderDomain(t *testing.T, vm *v1alpha1.VirtualMachine, hostArch string) *libvirtxml.Domain {
	t.Helper()
	xml, err := GenerateDomainXML(vm, hostArch)
	if err != nil {
		t.Fatalf("GenerateDomainXML() error = %v", err)
	}
	var d libvirtxml.Domain
	if err := d.Unmarshal(xml); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return &d
}

func TestGenerateDomainXML_NoFirmwareAutoSelects(t *testing.T) {
	for _, arch := range []string{"", v1alpha1.ArchAarch64} {
		d := renderDomain(t, archTestVM(arch), arch)
		if d.OS.Firmware != "efi" {
			t.Errorf("arch %q: firmware = %q, want efi", arch, d.OS.Firmware)
		}
		if d.OS.Loader != nil || d.OS.NVRam != nil {
			t.Errorf("arch %q: unexpected loader or nvram without spec.firmware", arch)
		}
		if d.Features.SMM != nil {
			t.Errorf("arch %q: unexpected SMM without spec.firmware", arch)
		}
	}
}

func TestGenerateDomainXML_Firmware(t *testing.T) {
	const (
		loader   = "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd"
		template = "/srv/firmware/custom_VARS.fd"
	)
	tests := []struct {
		name         string
		arch         string
		fw           v1alpha1.FirmwareSpec
		wantTemplate string
		wantSecure   bool
	}{
		{
			name: "loader only",
			fw:   v1alpha1.FirmwareSpec{Loader: loader},
		},
		{
			name:         "loader and template",
			fw:           v1alpha1.FirmwareSpec{Loader: loader, NVRAMTemplate: template},
			wantTemplate: template,
		},
		{
			name:         "secure boot",
			fw:           v1alpha1.FirmwareSpec{Loader: loader, SecureBoot: true, NVRAMTemplate: template},
			wantTemplate: template,
			wantSecure:   true,
		},
		{
			name:         "key profile",
			fw:           v1alpha1.FirmwareSpec{Loader: loader, SecureBoot: true, Keys: "my-keys"},
			wantTemplate: storage.DefaultImagesPath + "/varstore_my-keys_x86_64.fd",
			wantSecure:   true,
		},
		{
			name:         "aarch64 key profile",
			arch:         v1alpha1.ArchAarch64,
			fw:           v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI-silent-pflash.raw", Keys: "my-keys"},
			wantTemplate: storage.DefaultImagesPath + "/varstore_my-keys_aarch64.fd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := archTestVM(tt.arch)
			fw := tt.fw
			vm.Spec.Firmware = &fw
			d := renderDomain(t, vm, tt.arch)

			if d.OS.Firmware != "" {
				t.Errorf("firmware = %q, want no auto-selection", d.OS.Firmware)
			}
			l := d.OS.Loader
			if l == nil || l.Path != fw.Loader || l.Type != "pflash" || l.Readonly != "yes" {
				t.Fatalf("loader = %+v, want read-only pflash %s", l, fw.Loader)
			}
			if got := l.Secure == "yes"; got != tt.wantSecure {
				t.Errorf("loader secure = %q, want secure %v", l.Secure, tt.wantSecure)
			}
			if got := d.Features.SMM != nil && d.Features.SMM.State == "on"; got != tt.wantSecure {
				t.Errorf("SMM on = %v, want %v", got, tt.wantSecure)
			}
			switch {
			case tt.wantTemplate == "" && d.OS.NVRam != nil:
				t.Errorf("nvram = %+v, want none", d.OS.NVRam)
			case tt.wantTemplate != "" && (d.OS.NVRam == nil || d.OS.NVRam.Template != tt.wantTemplate):
				t.Errorf("nvram = %+v, want template %s", d.OS.NVRam, tt.wantTemplate)
			}
			if tt.arch == v1alpha1.ArchAarch64 && d.Features.GIC == nil {
				t.Error("aarch64 features lost the GIC")
			}
		})
	}
}
