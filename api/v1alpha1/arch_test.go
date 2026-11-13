package v1alpha1

import (
	"testing"
)

func TestArchFromGOARCH(t *testing.T) {
	tests := []struct {
		goarch   string
		expected string
	}{
		{"amd64", ArchX86_64},
		{"arm64", ArchAarch64},
		{"riscv64", "riscv64"},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			if got := ArchFromGOARCH(tt.goarch); got != tt.expected {
				t.Errorf("ArchFromGOARCH(%q) = %s, want %s", tt.goarch, got, tt.expected)
			}
		})
	}
}

func TestGetArch(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		expected string
	}{
		{"empty defaults to x86_64", "", ArchX86_64},
		{"x86_64", ArchX86_64, ArchX86_64},
		{"aarch64", ArchAarch64, ArchAarch64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{Spec: VirtualMachineSpec{Arch: tt.arch}}
			if got := vm.GetArch(); got != tt.expected {
				t.Errorf("Expected GetArch() = %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetDomainType(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		hostArch string
		expected string
	}{
		{"no arch on x86_64 host", "", ArchX86_64, DomainTypeKVM},
		{"no arch on aarch64 host stays kvm", "", ArchAarch64, DomainTypeKVM},
		{"no arch, host unknown", "", "", DomainTypeKVM},
		{"x86_64 on x86_64 host", ArchX86_64, ArchX86_64, DomainTypeKVM},
		{"aarch64 on aarch64 host", ArchAarch64, ArchAarch64, DomainTypeKVM},
		{"aarch64 on x86_64 host", ArchAarch64, ArchX86_64, DomainTypeQEMU},
		{"x86_64 on aarch64 host", ArchX86_64, ArchAarch64, DomainTypeQEMU},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{Spec: VirtualMachineSpec{Arch: tt.arch}}
			if got := vm.GetDomainType(tt.hostArch); got != tt.expected {
				t.Errorf("GetDomainType(%q) = %s, want %s", tt.hostArch, got, tt.expected)
			}
		})
	}
}

func TestCPUModeFor_ArchDefaults(t *testing.T) {
	tests := []struct {
		name       string
		arch       string
		domainType string
		cpuMode    string
		expected   string
	}{
		{"no arch kvm", "", DomainTypeKVM, "", CPUModeHostModel},
		{"x86_64 kvm", ArchX86_64, DomainTypeKVM, "", CPUModeHostModel},
		{"x86_64 qemu", ArchX86_64, DomainTypeQEMU, "", CPUModeMaximum},
		{"aarch64 kvm", ArchAarch64, DomainTypeKVM, "", CPUModeHostPassthrough},
		{"aarch64 qemu", ArchAarch64, DomainTypeQEMU, "", CPUModeMaximum},
		{"explicit mode wins", ArchAarch64, DomainTypeQEMU, CPUModeHostPassthrough, CPUModeHostPassthrough},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VirtualMachine{Spec: VirtualMachineSpec{Arch: tt.arch, CPUMode: tt.cpuMode}}
			if got := vm.CPUModeFor(tt.domainType); got != tt.expected {
				t.Errorf("CPUModeFor(%q) = %s, want %s", tt.domainType, got, tt.expected)
			}
		})
	}
}

func TestGetCPUMode_Aarch64(t *testing.T) {
	// GetCPUMode is the mode under KVM on the guest's own architecture.
	vm := &VirtualMachine{Spec: VirtualMachineSpec{Arch: ArchAarch64}}
	if got := vm.GetCPUMode(); got != CPUModeHostPassthrough {
		t.Errorf("GetCPUMode() = %s, want %s", got, CPUModeHostPassthrough)
	}
}
