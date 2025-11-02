package vm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/internal/storage"
)

func TestDestroyWithDeps_VMDoesNotExist(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	// Configure mock: VM does not exist
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return libvirt.Domain{}, fmt.Errorf("domain not found: %s", name)
	}

	// Execute
	err := destroyWithDeps(ctx, "nonexistent-vm", lv, sm)

	// Verify
	if err == nil {
		t.Fatal("expected error when VM doesn't exist, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Verify no other operations were attempted
	if len(lv.domainGetStateCalls) > 0 {
		t.Error("should not check state if VM lookup fails")
	}
	if len(lv.domainUndefineFlagsCalls) > 0 {
		t.Error("should not undefine if VM lookup fails")
	}
}

func TestDestroyWithDeps_RunningVM_GracefulShutdown(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists and is running, graceful shutdown works
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	callCount := 0
	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		callCount++
		// First call: running, subsequent calls: shutoff (simulates graceful shutdown)
		if callCount == 1 {
			return domainStateRunning, 0, nil
		}
		return domainStateShutoff, 0, nil
	}

	// Configure storage: return some volumes for this VM
	sm.listVolumesFunc = func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
		if poolName == "foundry-vms" {
			return []storage.VolumeInfo{
				{Name: "test-vm_boot", Pool: poolName},
				{Name: "test-vm_data-vdb", Pool: poolName},
				{Name: "test-vm_cloudinit", Pool: poolName},
			}, nil
		}
		return []storage.VolumeInfo{}, nil
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workflow
	if len(lv.domainLookupByNameCalls) != 1 {
		t.Errorf("expected 1 domain lookup, got %d", len(lv.domainLookupByNameCalls))
	}
	if len(lv.domainShutdownCalls) != 1 {
		t.Errorf("expected 1 shutdown call, got %d", len(lv.domainShutdownCalls))
	}
	if len(lv.domainDestroyCalls) != 0 {
		t.Errorf("expected 0 force destroy calls (graceful shutdown worked), got %d", len(lv.domainDestroyCalls))
	}
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("expected 1 undefine call, got %d", len(lv.domainUndefineFlagsCalls))
	}

	// Verify volume cleanup
	if len(sm.deleteVolumeCalls) != 3 {
		t.Errorf("expected 3 volume deletes, got %d", len(sm.deleteVolumeCalls))
	}
	expectedVolumes := map[string]bool{
		"foundry-vms/test-vm_boot":      true,
		"foundry-vms/test-vm_data-vdb":  true,
		"foundry-vms/test-vm_cloudinit": true,
	}
	for _, vol := range sm.deleteVolumeCalls {
		if !expectedVolumes[vol] {
			t.Errorf("unexpected volume deleted: %s", vol)
		}
	}
}

func TestDestroyWithDeps_RunningVM_ForceDestroy(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists and is running, graceful shutdown fails
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	// Always return running state (graceful shutdown doesn't work)
	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateRunning, 0, nil
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workflow
	if len(lv.domainShutdownCalls) != 1 {
		t.Errorf("expected 1 shutdown call, got %d", len(lv.domainShutdownCalls))
	}
	if len(lv.domainDestroyCalls) != 1 {
		t.Errorf("expected 1 force destroy call, got %d", len(lv.domainDestroyCalls))
	}
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("expected 1 undefine call, got %d", len(lv.domainUndefineFlagsCalls))
	}
}

func TestDestroyWithDeps_StoppedVM(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists but is already stopped
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateShutoff, 0, nil
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workflow: should skip shutdown/destroy
	if len(lv.domainShutdownCalls) != 0 {
		t.Errorf("expected 0 shutdown calls (VM already stopped), got %d", len(lv.domainShutdownCalls))
	}
	if len(lv.domainDestroyCalls) != 0 {
		t.Errorf("expected 0 destroy calls (VM already stopped), got %d", len(lv.domainDestroyCalls))
	}
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("expected 1 undefine call, got %d", len(lv.domainUndefineFlagsCalls))
	}
}

func TestDestroyWithDeps_UndefineFails(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists, stopped, but undefine fails
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateShutoff, 0, nil
	}

	lv.domainUndefineFlagsFunc = func(dom libvirt.Domain, flags libvirt.DomainUndefineFlagsValues) error {
		return fmt.Errorf("undefine failed: permission denied")
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify: should return error
	if err == nil {
		t.Fatal("expected error when undefine fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to undefine") {
		t.Errorf("expected 'failed to undefine' error, got: %v", err)
	}

	// Volume cleanup should not happen if undefine fails
	if len(sm.deleteVolumeCalls) > 0 {
		t.Error("should not delete volumes if undefine fails")
	}
}

func TestDestroyWithDeps_VolumeCleanupBestEffort(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists and is stopped
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateShutoff, 0, nil
	}

	// Configure storage: return volumes, but deletion fails for one
	sm.listVolumesFunc = func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
		if poolName == "foundry-vms" {
			return []storage.VolumeInfo{
				{Name: "test-vm_boot", Pool: poolName},
				{Name: "test-vm_cloudinit", Pool: poolName},
			}, nil
		}
		return []storage.VolumeInfo{}, nil
	}

	deleteCount := 0
	sm.deleteVolumeFunc = func(ctx context.Context, poolName, volumeName string) error {
		deleteCount++
		if volumeName == "test-vm_boot" {
			return fmt.Errorf("delete failed: volume in use")
		}
		return nil
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify: should succeed despite volume deletion failure (best-effort)
	if err != nil {
		t.Fatalf("unexpected error (volume cleanup is best-effort): %v", err)
	}

	// Verify both volumes were attempted
	if len(sm.deleteVolumeCalls) != 2 {
		t.Errorf("expected 2 volume delete attempts, got %d", len(sm.deleteVolumeCalls))
	}
}

func TestDestroyWithDeps_OnlyDeletesMatchingVolumes(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "my-vm"}

	// Configure mock: VM exists and is stopped
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateShutoff, 0, nil
	}

	// Configure storage: return volumes for multiple VMs
	sm.listVolumesFunc = func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
		if poolName == "foundry-vms" {
			return []storage.VolumeInfo{
				{Name: "my-vm_boot", Pool: poolName},        // Should delete
				{Name: "my-vm_data-vdb", Pool: poolName},    // Should delete
				{Name: "other-vm_boot", Pool: poolName},     // Should NOT delete
				{Name: "my-vm-backup_boot", Pool: poolName}, // Should NOT delete (different prefix)
				{Name: "my-vm_cloudinit", Pool: poolName},   // Should delete
			}, nil
		}
		return []storage.VolumeInfo{}, nil
	}

	// Execute
	err := destroyWithDeps(ctx, "my-vm", lv, sm)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only matching volumes were deleted
	if len(sm.deleteVolumeCalls) != 3 {
		t.Errorf("expected 3 volume deletes, got %d", len(sm.deleteVolumeCalls))
	}

	expectedVolumes := map[string]bool{
		"foundry-vms/my-vm_boot":      true,
		"foundry-vms/my-vm_data-vdb":  true,
		"foundry-vms/my-vm_cloudinit": true,
	}
	for _, vol := range sm.deleteVolumeCalls {
		if !expectedVolumes[vol] {
			t.Errorf("unexpected volume deleted: %s", vol)
		}
	}

	// Verify non-matching volumes were NOT deleted
	for _, vol := range sm.deleteVolumeCalls {
		if strings.Contains(vol, "other-vm") || strings.Contains(vol, "backup") {
			t.Errorf("should not delete volumes from other VMs: %s", vol)
		}
	}
}

func TestDestroyWithDeps_ListVolumesFailure(t *testing.T) {
	ctx := context.Background()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	testDomain := libvirt.Domain{Name: "test-vm"}

	// Configure mock: VM exists and is stopped
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return testDomain, nil
	}

	lv.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return domainStateShutoff, 0, nil
	}

	// Configure storage: list fails
	sm.listVolumesFunc = func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
		return nil, fmt.Errorf("pool not found")
	}

	// Execute
	err := destroyWithDeps(ctx, "test-vm", lv, sm)

	// Verify: should succeed (volume cleanup is best-effort)
	if err != nil {
		t.Fatalf("unexpected error (volume listing failure should be tolerated): %v", err)
	}

	// VM should be undefined even though volume cleanup failed
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("expected 1 undefine call, got %d", len(lv.domainUndefineFlagsCalls))
	}
}
