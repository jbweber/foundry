package vm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestCreateFromConfigWithDeps_Arch(t *testing.T) {
	tests := []struct {
		name         string
		arch         string
		hostArch     string
		wantCapCalls int
		wantType     string
		wantArch     string
	}{
		// A spec without arch never asks libvirt for the host's architecture
		{name: "no arch", hostArch: v1alpha1.ArchAarch64, wantCapCalls: 0, wantType: `type="kvm"`, wantArch: `arch="x86_64"`},
		{name: "aarch64 on aarch64", arch: v1alpha1.ArchAarch64, hostArch: v1alpha1.ArchAarch64, wantCapCalls: 1, wantType: `type="kvm"`, wantArch: `arch="aarch64"`},
		{name: "aarch64 on x86_64", arch: v1alpha1.ArchAarch64, hostArch: v1alpha1.ArchX86_64, wantCapCalls: 1, wantType: `type="qemu"`, wantArch: `arch="aarch64"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := newMockLibvirtClient()
			lv.connectGetCapabilitiesFunc = func() (string, error) {
				return `<capabilities><host><cpu><arch>` + tt.hostArch + `</arch></cpu></host></capabilities>`, nil
			}
			sm := newMockStorageManager()
			vm := testVMConfig()
			vm.Spec.Arch = tt.arch

			if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if lv.connectGetCapabilitiesCalls != tt.wantCapCalls {
				t.Errorf("ConnectGetCapabilities calls = %d, want %d", lv.connectGetCapabilitiesCalls, tt.wantCapCalls)
			}
			if len(lv.domainDefineXMLCalls) != 1 {
				t.Fatalf("DomainDefineXML calls = %d, want 1", len(lv.domainDefineXMLCalls))
			}
			xml := lv.domainDefineXMLCalls[0]
			for _, want := range []string{tt.wantType, tt.wantArch} {
				if !strings.Contains(xml, want) {
					t.Errorf("domain XML missing %s:\n%s", want, xml)
				}
			}
		})
	}
}

func TestCreateFromConfigWithDeps_HostArchFailure(t *testing.T) {
	lv := newMockLibvirtClient()
	lv.connectGetCapabilitiesFunc = func() (string, error) {
		return "", errors.New("connection lost")
	}
	sm := newMockStorageManager()
	vm := testVMConfig()
	vm.Spec.Arch = v1alpha1.ArchAarch64

	err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "hypervisor architecture") {
		t.Fatalf("error = %v, want a hypervisor architecture error", err)
	}
	if len(lv.domainDefineXMLCalls) != 0 {
		t.Error("domain defined despite the failure")
	}
}
