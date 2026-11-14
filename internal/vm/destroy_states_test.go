package vm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
)

// TestDestroyWithDeps_StopsEveryActiveState: destroy stops a domain in any
// state but shut off before it undefines it, with or without force.
// Paused, suspended and crashed guests cannot answer the ACPI power button
// and are destroyed at once; the others get a graceful shutdown first.
func TestDestroyWithDeps_StopsEveryActiveState(t *testing.T) {
	shortShutdown(t)
	tests := []struct {
		name         string
		state        libvirt.DomainState
		wantShutdown bool
	}{
		{"no state", libvirt.DomainNostate, true},
		{"running", libvirt.DomainRunning, true},
		{"blocked", libvirt.DomainBlocked, true},
		{"paused", libvirt.DomainPaused, false},
		{"shutting down", libvirt.DomainShutdown, true},
		{"crashed", libvirt.DomainCrashed, false},
		{"pmsuspended", libvirt.DomainPmsuspended, false},
	}
	for _, tt := range tests {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/force=%v", tt.name, force), func(t *testing.T) {
				vm := testVMConfig()
				lv, sm := poolMocks("foundry-vms/test-vm_boot.qcow2")
				if force {
					noSpec(lv)
				} else {
					withSpec(t, lv, vm)
				}
				destroyed := false
				lv.domainDestroyFunc = func(libvirt.Domain) error { destroyed = true; return nil }
				// The guest ignores the power button; only a destroy stops it.
				lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) {
					if destroyed {
						return domainStateShutoff, 0, nil
					}
					return int32(tt.state), 0, nil
				}

				if err := destroyWithDeps(context.Background(), vm.Name, lv, sm, force); err != nil {
					t.Fatalf("destroy failed: %v", err)
				}
				if got := len(lv.domainShutdownCalls) == 1; got != tt.wantShutdown {
					t.Errorf("graceful shutdown tried: %v, want %v", got, tt.wantShutdown)
				}
				if len(lv.domainDestroyCalls) != 1 {
					t.Errorf("expected 1 force destroy, got %d", len(lv.domainDestroyCalls))
				}
				if len(lv.domainUndefineFlagsCalls) != 1 || len(sm.deleteVolumeCalls) != 1 {
					t.Errorf("expected the domain undefined and its volume deleted: undefines %d, deleted %v",
						len(lv.domainUndefineFlagsCalls), sm.deleteVolumeCalls)
				}
			})
		}
	}
}

// TestDestroyWithDeps_CannotStop: a domain that cannot be stopped, or whose
// state cannot be read, is an error, with or without force, and its
// definition and volumes stay: undefining it would leave it running as a
// transient domain with its disks deleted under it.
func TestDestroyWithDeps_CannotStop(t *testing.T) {
	shortShutdown(t)
	busy := errors.New("Failed to terminate process: Device or resource busy")
	tests := []struct {
		name  string
		setup func(lv *mockLibvirtClient)
	}{
		{"running, force destroy fails", func(lv *mockLibvirtClient) {
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateRunning, 0, nil }
			lv.domainShutdownFunc = func(libvirt.Domain) error { return errors.New("guest agent not responding") }
			lv.domainDestroyFunc = func(libvirt.Domain) error { return busy }
		}},
		{"paused, force destroy fails", func(lv *mockLibvirtClient) {
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return int32(libvirt.DomainPaused), 0, nil }
			lv.domainDestroyFunc = func(libvirt.Domain) error { return busy }
		}},
		{"state unreadable", func(lv *mockLibvirtClient) {
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return 0, 0, errors.New("connection reset") }
		}},
		{"state unreadable after shutdown", func(lv *mockLibvirtClient) {
			calls := 0
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) {
				calls++
				if calls == 1 {
					return domainStateRunning, 0, nil
				}
				return 0, 0, errors.New("connection reset")
			}
		}},
	}
	for _, tt := range tests {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/force=%v", tt.name, force), func(t *testing.T) {
				vm := testVMConfig()
				lv, sm := poolMocks("foundry-vms/test-vm_boot.qcow2")
				if force {
					noSpec(lv)
				} else {
					withSpec(t, lv, vm)
				}
				tt.setup(lv)

				if err := destroyWithDeps(context.Background(), vm.Name, lv, sm, force); err == nil {
					t.Fatal("expected an error")
				}
				if len(lv.domainUndefineFlagsCalls) != 0 || len(lv.domainUndefineCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
					t.Errorf("nothing may change: undefines %d, deleted %v",
						len(lv.domainUndefineFlagsCalls)+len(lv.domainUndefineCalls), sm.deleteVolumeCalls)
				}
			})
		}
	}
}

// shortShutdown makes destroy's graceful-shutdown wait short for one test.
func shortShutdown(t *testing.T) {
	old := shutdownTimeout
	shutdownTimeout = 600 * time.Millisecond
	t.Cleanup(func() { shutdownTimeout = old })
}
