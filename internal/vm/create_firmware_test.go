package vm

import (
	"context"
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

func TestCreateFromConfigWithDeps_KeyProfile(t *testing.T) {
	const varstore = "varstore_my-keys_x86_64.fd"
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	sm.volumeExistsFunc = func(ctx context.Context, poolName, volumeName string) (bool, error) {
		return poolName == storage.DefaultImagesPool && volumeName == varstore, nil
	}
	sm.getImagePathFunc = func(ctx context.Context, imageName string) (string, error) {
		return "/pool/images/" + imageName, nil
	}
	vm := testVMConfig()
	vm.Spec.Firmware = &v1alpha1.FirmwareSpec{
		Loader:     "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd",
		SecureBoot: true,
		Keys:       "my-keys",
	}

	if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(lv.domainDefineXMLCalls) != 1 {
		t.Fatalf("DomainDefineXML calls = %d, want 1", len(lv.domainDefineXMLCalls))
	}
	xml := lv.domainDefineXMLCalls[0]
	for _, want := range []string{`template="/pool/images/` + varstore + `"`, `secure="yes"`, `<smm state="on">`} {
		if !strings.Contains(xml, want) {
			t.Errorf("domain XML missing %s:\n%s", want, xml)
		}
	}
	// The caller's spec keeps the profile name, not the resolved path.
	if vm.Spec.Firmware.Keys != "my-keys" || vm.Spec.Firmware.NVRAMTemplate != "" {
		t.Errorf("spec.firmware changed: %+v", vm.Spec.Firmware)
	}
}

func TestCreateFromConfigWithDeps_MissingKeyProfile(t *testing.T) {
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	vm := testVMConfig()
	vm.Spec.Arch = v1alpha1.ArchAarch64
	vm.Spec.CPUMode = v1alpha1.CPUModeMaximum
	vm.Spec.Firmware = &v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI-silent-pflash.raw", Keys: "my-keys"}

	err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "has no aarch64 varstore") {
		t.Fatalf("error = %v, want a missing aarch64 varstore error", err)
	}
	if len(sm.createVolumeCalls) != 0 || len(lv.domainDefineXMLCalls) != 0 {
		t.Error("storage or domain created despite the missing key profile")
	}
}
