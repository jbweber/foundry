package vm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

// testVMConfig creates a minimal valid VM config for testing
func testVMConfig() *v1alpha1.VirtualMachine {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "test-vm",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			MemoryGiB:   2,
			VCPUs:       2,
			StoragePool: "foundry-vms", // Set explicitly for tests
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB:    20,
				Empty:     true, // Create empty disk for testing
				ImagePool: "foundry-images",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					Bridge:       "br0",
					IP:           "10.0.0.10/24",
					Gateway:      "10.0.0.1",
					DefaultRoute: true,
				},
			},
		},
	}
	return vm
}

// testVMConfigWithCloudInit creates a config with cloud-init for testing
func testVMConfigWithCloudInit() *v1alpha1.VirtualMachine {
	vm := testVMConfig()
	vm.Spec.CloudInit = &v1alpha1.CloudInitSpec{
		FQDN: "test-vm.example.com",
		SSHAuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIbJKZscbOLzBsgY5y2QupKW4A2kSDjMBQGPb1dChr+S test@example.com",
		},
		PasswordHash: "$6$rounds=656000$test",
	}
	return vm
}

// testVMConfigWithDataDisks creates a config with data disks for testing
func testVMConfigWithDataDisks() *v1alpha1.VirtualMachine {
	vm := testVMConfig()
	vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{
		{Device: "vdb", SizeGB: 50},
		{Device: "vdc", SizeGB: 100},
	}
	return vm
}

// TestCreateFromConfigWithDeps_Success tests the happy path
func TestCreateFromConfigWithDeps_Success(t *testing.T) {
	tests := []struct {
		name string
		vm   *v1alpha1.VirtualMachine
	}{
		{"minimal config", testVMConfig()},
		{"with cloud-init", testVMConfigWithCloudInit()},
		{"with data disks", testVMConfigWithDataDisks()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()

			err := createFromConfigWithDeps(ctx, tt.vm, lv, sm)
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}

			// Verify no cleanup was called (success path)
			if len(lv.domainUndefineCalls) > 0 {
				t.Error("unexpected cleanup: domain undefine called on success")
			}
			if len(sm.deleteVolumeCalls) > 0 {
				t.Error("unexpected cleanup: storage delete called on success")
			}

			// Verify volumes were created
			if len(sm.createVolumeCalls) == 0 {
				t.Error("expected at least boot volume to be created")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_PreflightChecksFail tests early failures before resource creation
func TestCreateFromConfigWithDeps_PreflightChecksFail(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*mockLibvirtClient, *mockStorageManager)
		expectError   string
		expectCleanup bool
	}{
		{
			name: "VM already exists",
			setupMock: func(lv *mockLibvirtClient, sm *mockStorageManager) {
				lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
					return libvirt.Domain{Name: name}, nil
				}
			},
			expectError:   "already exists",
			expectCleanup: false,
		},
		{
			name: "boot volume already exists",
			setupMock: func(lv *mockLibvirtClient, sm *mockStorageManager) {
				sm.volumeExistsFunc = func(ctx context.Context, poolName, volumeName string) (bool, error) {
					return true, nil
				}
			},
			expectError:   "boot volume already exists",
			expectCleanup: false,
		},
		{
			name: "backing image not found",
			setupMock: func(lv *mockLibvirtClient, sm *mockStorageManager) {
				sm.imageExistsFunc = func(ctx context.Context, imageName string) (bool, error) {
					return false, nil
				}
			},
			expectError:   "backing image not found",
			expectCleanup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfig()
			// For backing image test, we need an image reference
			if tt.name == "backing image not found" {
				vm.Spec.BootDisk.Image = "fedora-43"
				vm.Spec.BootDisk.Empty = false
			}
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			tt.setupMock(lv, sm)

			err := createFromConfigWithDeps(ctx, vm, lv, sm)

			// Verify error occurred
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got: %v", tt.expectError, err)
			}

			// Verify no volumes were created (preflight checks fail early)
			if len(sm.createVolumeCalls) > 0 {
				t.Error("unexpected volume creation on preflight failure")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_StorageFailures tests failures during storage creation
func TestCreateFromConfigWithDeps_StorageFailures(t *testing.T) {
	tests := []struct {
		name          string
		vm            *v1alpha1.VirtualMachine
		setupMock     func(*mockStorageManager)
		expectCleanup bool
	}{
		{
			name: "create boot volume fails",
			vm:   testVMConfig(),
			setupMock: func(sm *mockStorageManager) {
				sm.createVolumeFunc = func(ctx context.Context, poolName string, spec storage.VolumeSpec) error {
					return errors.New("libvirt create volume failed")
				}
			},
			expectCleanup: false, // storageCreated flag not set yet
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			tt.setupMock(sm)

			err := createFromConfigWithDeps(ctx, tt.vm, lv, sm)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// Verify cleanup behavior
			if tt.expectCleanup {
				if len(sm.deleteVolumeCalls) == 0 {
					t.Error("expected storage cleanup")
				}
			} else {
				if len(sm.deleteVolumeCalls) > 0 {
					t.Error("unexpected storage cleanup")
				}
			}

			// Domain should never be defined if storage fails
			if len(lv.domainDefineXMLCalls) > 0 {
				t.Error("unexpected domain define on storage failure")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_LibvirtFailures tests failures during libvirt operations
func TestCreateFromConfigWithDeps_LibvirtFailures(t *testing.T) {
	tests := []struct {
		name                string
		setupMock           func(*mockLibvirtClient)
		expectDomainDefined bool
	}{
		{
			name: "define domain fails",
			setupMock: func(lv *mockLibvirtClient) {
				lv.domainDefineXMLFunc = func(xml string) (libvirt.Domain, error) {
					return libvirt.Domain{}, errors.New("invalid XML")
				}
			},
			expectDomainDefined: false,
		},
		{
			name: "set autostart fails",
			setupMock: func(lv *mockLibvirtClient) {
				lv.domainSetAutostartFunc = func(dom libvirt.Domain, autostart int32) error {
					return errors.New("permission denied")
				}
			},
			expectDomainDefined: true,
		},
		{
			name: "start domain fails",
			setupMock: func(lv *mockLibvirtClient) {
				lv.domainCreateFunc = func(dom libvirt.Domain) error {
					return errors.New("not enough resources")
				}
			},
			expectDomainDefined: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfig()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			tt.setupMock(lv)

			err := createFromConfigWithDeps(ctx, vm, lv, sm)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// Verify storage cleanup always happens (at least boot volume)
			if len(sm.deleteVolumeCalls) == 0 {
				t.Error("expected storage cleanup")
			}

			// Verify domain cleanup only happens if domain was defined
			if tt.expectDomainDefined {
				if len(lv.domainUndefineCalls) != 1 {
					t.Errorf("expected domain cleanup, got %d calls", len(lv.domainUndefineCalls))
				}
			} else {
				if len(lv.domainUndefineCalls) > 0 {
					t.Error("unexpected domain cleanup when define failed")
				}
			}
		})
	}
}

// TestCleanupWithDeps tests the cleanup function behavior
func TestCleanupWithDeps(t *testing.T) {
	tests := []struct {
		name                 string
		domainDefined        bool
		storageCreated       bool
		expectDomainCleanup  bool
		expectStorageCleanup bool
	}{
		{
			name:                 "both created",
			domainDefined:        true,
			storageCreated:       true,
			expectDomainCleanup:  true,
			expectStorageCleanup: true,
		},
		{
			name:                 "only domain created",
			domainDefined:        true,
			storageCreated:       false,
			expectDomainCleanup:  true,
			expectStorageCleanup: false,
		},
		{
			name:                 "only storage created",
			domainDefined:        false,
			storageCreated:       true,
			expectDomainCleanup:  false,
			expectStorageCleanup: true,
		},
		{
			name:                 "nothing created",
			domainDefined:        false,
			storageCreated:       false,
			expectDomainCleanup:  false,
			expectStorageCleanup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfig()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()

			// If domain was defined, simulate that by making lookup succeed
			if tt.domainDefined {
				lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
					return libvirt.Domain{Name: name}, nil
				}
			}

			cleanupWithDeps(ctx, vm, sm, lv, tt.domainDefined, tt.storageCreated)

			// Verify cleanup behavior
			if tt.expectDomainCleanup {
				if len(lv.domainUndefineCalls) != 1 {
					t.Errorf("expected domain cleanup, got %d calls", len(lv.domainUndefineCalls))
				}
			} else {
				if len(lv.domainUndefineCalls) > 0 {
					t.Error("unexpected domain cleanup")
				}
			}

			if tt.expectStorageCleanup {
				// Should delete at least boot volume
				if len(sm.deleteVolumeCalls) == 0 {
					t.Error("expected storage cleanup")
				}
			} else {
				if len(sm.deleteVolumeCalls) > 0 {
					t.Error("unexpected storage cleanup")
				}
			}
		})
	}
}

// TestCleanupWithDeps_ContinuesOnError tests that cleanup continues even if operations fail
func TestCleanupWithDeps_ContinuesOnError(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfig()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	// Make all cleanup operations fail
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		return libvirt.Domain{}, errors.New("lookup failed")
	}
	sm.deleteVolumeFunc = func(ctx context.Context, poolName, volumeName string) error {
		return errors.New("delete failed")
	}

	// Should not panic
	cleanupWithDeps(ctx, vm, sm, lv, true, true)

	// Verify attempts were made despite failures
	if len(lv.domainLookupByNameCalls) != 1 {
		t.Error("expected domain cleanup attempt")
	}
	if len(sm.deleteVolumeCalls) == 0 {
		t.Error("expected storage cleanup attempt")
	}
}
