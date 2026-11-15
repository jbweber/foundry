package vm

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func uefiVarsVM(secureBoot bool) *v1alpha1.VirtualMachine {
	vm := testVMConfig()
	vm.Spec.Arch = v1alpha1.ArchAarch64
	vm.Spec.Firmware = &v1alpha1.FirmwareSpec{Loader: "/usr/share/edk2/aarch64/QEMU_EFI.qemuvars.fd", SecureBoot: secureBoot, Keys: "corp-keys"}
	return vm
}

// An aarch64 Secure Boot VM asks libvirt's domain capabilities, for its own
// architecture, machine and domain type, and is refused on a host without the
// uefi-vars device; any other VM makes no such call.
func TestCheckUEFIVarsSupport(t *testing.T) {
	lv := newMockLibvirtClient()
	if err := checkUEFIVarsSupport(uefiVarsVM(true), lv); err != nil {
		t.Fatalf("supported host: %v", err)
	}
	// The mock host is x86_64, so an aarch64 guest is emulated there
	if len(lv.domainCapabilitiesCalls) != 1 || lv.domainCapabilitiesCalls[0] != "aarch64/virt/qemu" {
		t.Errorf("asked for %v", lv.domainCapabilitiesCalls)
	}
	lv.domainCapabilitiesFunc = func(arch, _, _ string) (string, error) {
		return `<domainCapabilities><arch>` + arch + `</arch><os supported='yes'/></domainCapabilities>`, nil
	}
	if err := checkUEFIVarsSupport(uefiVarsVM(true), lv); err == nil || !strings.Contains(err.Error(), "uefi-vars") {
		t.Errorf("unsupported host: %v", err)
	}
	lv.domainCapabilitiesCalls = nil
	for _, vm := range []*v1alpha1.VirtualMachine{uefiVarsVM(false), testVMConfig()} {
		if err := checkUEFIVarsSupport(vm, lv); err != nil {
			t.Error(err)
		}
	}
	if len(lv.domainCapabilitiesCalls) != 0 {
		t.Errorf("a VM without aarch64 Secure Boot asked libvirt: %v", lv.domainCapabilitiesCalls)
	}
}
