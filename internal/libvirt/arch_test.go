package libvirt

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// TestMachineType: each supported architecture has its machine, and any other
// is refused rather than given x86_64's.
func TestMachineType(t *testing.T) {
	for arch, want := range map[string]string{v1alpha1.ArchX86_64: "q35", v1alpha1.ArchAarch64: "virt"} {
		got, err := MachineType(arch)
		if err != nil || got != want {
			t.Errorf("MachineType(%s) = %q, %v; want %q", arch, got, err, want)
		}
	}
	for _, arch := range []string{"", "riscv64", "ppc64le", "arm64", "amd64"} {
		if got, err := MachineType(arch); err == nil {
			t.Errorf("MachineType(%q) = %q, want an error", arch, got)
		}
	}
}

// TestUnsupportedArchRefused: a domain for an architecture with no settings
// is an error, not an x86_64 definition.
func TestUnsupportedArchRefused(t *testing.T) {
	vm := archTestVM("riscv64")
	if xml, err := GenerateDomainXML(vm, "riscv64"); err == nil || !strings.Contains(err.Error(), "riscv64") {
		t.Errorf("GenerateDomainXML: %v, want an error naming riscv64:\n%s", err, xml)
	}
}

// TestArchSettingsComplete: every architecture the loader accepts has
// settings, so spec validation and rendering agree on what is supported.
func TestArchSettingsComplete(t *testing.T) {
	for _, arch := range []string{v1alpha1.ArchX86_64, v1alpha1.ArchAarch64} {
		s, err := settingsFor(arch)
		if err != nil {
			t.Fatal(err)
		}
		if s.machine == "" || s.features == nil || s.timers == nil || s.cdromBus == "" {
			t.Errorf("%s: incomplete settings %+v", arch, s)
		}
	}
	if len(archs) != 2 {
		t.Errorf("archs has %d entries; a new one needs its golden cases (testdata/domains)", len(archs))
	}
}
