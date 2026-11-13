package loader

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestApplyDefaults_Arch(t *testing.T) {
	tests := []struct {
		name        string
		spec        v1alpha1.VirtualMachineSpec
		wantArch    string
		wantCPUMode string
	}{
		{
			// Unchanged from before spec.arch existed
			name:        "no arch",
			wantArch:    "",
			wantCPUMode: v1alpha1.CPUModeHostModel,
		},
		{
			// Picked at XML generation, from the hypervisor's architecture
			name:        "aarch64 leaves cpuMode unset",
			spec:        v1alpha1.VirtualMachineSpec{Arch: v1alpha1.ArchAarch64},
			wantArch:    v1alpha1.ArchAarch64,
			wantCPUMode: "",
		},
		{
			name:        "explicit cpuMode kept",
			spec:        v1alpha1.VirtualMachineSpec{Arch: v1alpha1.ArchAarch64, CPUMode: v1alpha1.CPUModeMaximum},
			wantArch:    v1alpha1.ArchAarch64,
			wantCPUMode: v1alpha1.CPUModeMaximum,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &v1alpha1.VirtualMachine{Spec: tt.spec}
			applyDefaults(vm)

			if vm.Spec.Arch != tt.wantArch {
				t.Errorf("Arch = %q, want %q", vm.Spec.Arch, tt.wantArch)
			}
			if vm.Spec.CPUMode != tt.wantCPUMode {
				t.Errorf("CPUMode = %q, want %q", vm.Spec.CPUMode, tt.wantCPUMode)
			}
		})
	}
}

func TestValidateSpec_Arch(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(spec *v1alpha1.VirtualMachineSpec)
		wantErr string
	}{
		{name: "no arch", mutate: func(spec *v1alpha1.VirtualMachineSpec) {}},
		{name: "x86_64", mutate: func(spec *v1alpha1.VirtualMachineSpec) { spec.Arch = v1alpha1.ArchX86_64 }},
		{name: "aarch64", mutate: func(spec *v1alpha1.VirtualMachineSpec) { spec.Arch = v1alpha1.ArchAarch64 }},
		{
			name:    "unknown arch",
			mutate:  func(spec *v1alpha1.VirtualMachineSpec) { spec.Arch = "riscv64" },
			wantErr: `spec.arch "riscv64" is not supported`,
		},
		{
			name:    "Go name refused",
			mutate:  func(spec *v1alpha1.VirtualMachineSpec) { spec.Arch = "arm64" },
			wantErr: `spec.arch "arm64" is not supported`,
		},
		{
			name: "aarch64 host-model refused",
			mutate: func(spec *v1alpha1.VirtualMachineSpec) {
				spec.Arch = v1alpha1.ArchAarch64
				spec.CPUMode = v1alpha1.CPUModeHostModel
			},
			wantErr: "host-model is not supported for aarch64",
		},
		{
			name: "aarch64 maximum",
			mutate: func(spec *v1alpha1.VirtualMachineSpec) {
				spec.Arch = v1alpha1.ArchAarch64
				spec.CPUMode = v1alpha1.CPUModeMaximum
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &v1alpha1.VirtualMachine{
				ObjectMeta: v1alpha1.ObjectMeta{Name: "arch-vm"},
				Spec: v1alpha1.VirtualMachineSpec{
					VCPUs:     2,
					MemoryGiB: 4,
					BootDisk:  v1alpha1.BootDiskSpec{SizeGB: 20, Image: "fedora-43.qcow2"},
					NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
						{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
					},
				},
			}
			tt.mutate(&vm.Spec)

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

func TestLoadFromYAML_Arch(t *testing.T) {
	yaml := `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: arm-vm
spec:
  arch: aarch64
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 20
    image: fedora-43-aarch64.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
`

	vm, err := LoadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	if vm.Spec.Arch != v1alpha1.ArchAarch64 {
		t.Errorf("Arch = %q, want %q", vm.Spec.Arch, v1alpha1.ArchAarch64)
	}
}
