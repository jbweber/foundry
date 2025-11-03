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

// TestParseImageReference tests the image reference parsing logic
func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name           string
		bootDisk       v1alpha1.BootDiskSpec
		wantPool       string
		wantVolume     string
		wantIsFilePath bool
		wantErr        bool
	}{
		{
			name: "empty image",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "",
			},
			wantPool:       "",
			wantVolume:     "",
			wantIsFilePath: false,
			wantErr:        false,
		},
		{
			name: "volume name only - with ImagePool",
			bootDisk: v1alpha1.BootDiskSpec{
				Image:     "fedora-43.qcow2",
				ImagePool: "custom-images",
			},
			wantPool:       "custom-images",
			wantVolume:     "fedora-43.qcow2",
			wantIsFilePath: false,
			wantErr:        false,
		},
		{
			name: "volume name only - default pool",
			bootDisk: v1alpha1.BootDiskSpec{
				Image:     "fedora-43.qcow2",
				ImagePool: "",
			},
			wantPool:       "foundry-images",
			wantVolume:     "fedora-43.qcow2",
			wantIsFilePath: false,
			wantErr:        false,
		},
		{
			name: "pool:volume format",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "foundry-images:fedora-43.qcow2",
			},
			wantPool:       "foundry-images",
			wantVolume:     "fedora-43.qcow2",
			wantIsFilePath: false,
			wantErr:        false,
		},
		{
			name: "pool:volume format with spaces",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "  my-pool  :  my-image.qcow2  ",
			},
			wantPool:       "my-pool",
			wantVolume:     "my-image.qcow2",
			wantIsFilePath: false,
			wantErr:        false,
		},
		{
			name: "absolute file path",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "/var/lib/libvirt/images/fedora.qcow2",
			},
			wantPool:       "",
			wantVolume:     "",
			wantIsFilePath: true,
			wantErr:        false,
		},
		{
			name: "relative file path",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "./images/fedora.qcow2",
			},
			wantPool:       "",
			wantVolume:     "",
			wantIsFilePath: true,
			wantErr:        false,
		},
		{
			name: "file path with subdirs",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "/some/path/to/image.qcow2",
			},
			wantPool:       "",
			wantVolume:     "",
			wantIsFilePath: true,
			wantErr:        false,
		},
		{
			name: "invalid pool:volume - empty pool",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: ":volume.qcow2",
			},
			wantErr: true,
		},
		{
			name: "invalid pool:volume - empty volume",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "pool:",
			},
			wantErr: true,
		},
		{
			name: "invalid pool:volume - empty both",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: ":",
			},
			wantErr: true,
		},
		{
			name: "pool:volume with special chars in volume name",
			bootDisk: v1alpha1.BootDiskSpec{
				Image: "pool:image-name_v1.2.qcow2",
			},
			wantPool:       "pool",
			wantVolume:     "image-name_v1.2.qcow2",
			wantIsFilePath: false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, volume, isFilePath, err := parseImageReference(tt.bootDisk)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if pool != tt.wantPool {
				t.Errorf("pool: got %q, want %q", pool, tt.wantPool)
			}
			if volume != tt.wantVolume {
				t.Errorf("volume: got %q, want %q", volume, tt.wantVolume)
			}
			if isFilePath != tt.wantIsFilePath {
				t.Errorf("isFilePath: got %v, want %v", isFilePath, tt.wantIsFilePath)
			}
		})
	}
}

// TestGetStoragePool tests the storage pool helper
func TestGetStoragePool(t *testing.T) {
	tests := []struct {
		name string
		vm   *v1alpha1.VirtualMachine
		want string
	}{
		{
			name: "explicit storage pool",
			vm: &v1alpha1.VirtualMachine{
				Spec: v1alpha1.VirtualMachineSpec{
					StoragePool: "custom-pool",
				},
			},
			want: "custom-pool",
		},
		{
			name: "default storage pool",
			vm: &v1alpha1.VirtualMachine{
				Spec: v1alpha1.VirtualMachineSpec{
					StoragePool: "",
				},
			},
			want: "foundry-vms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStoragePool(tt.vm)
			if got != tt.want {
				t.Errorf("getStoragePool() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestVolumeNaming tests volume naming helpers
func TestVolumeNaming(t *testing.T) {
	vm := &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "test-vm",
		},
	}

	t.Run("boot volume", func(t *testing.T) {
		got := getBootVolumeName(vm)
		want := "test-vm_boot.qcow2"
		if got != want {
			t.Errorf("getBootVolumeName() = %q, want %q", got, want)
		}
	})

	t.Run("data volume", func(t *testing.T) {
		got := getDataVolumeName(vm, "vdb")
		want := "test-vm_data-vdb.qcow2"
		if got != want {
			t.Errorf("getDataVolumeName() = %q, want %q", got, want)
		}
	})

	t.Run("cloud-init volume", func(t *testing.T) {
		got := getCloudInitVolumeName(vm)
		want := "test-vm_cloudinit.iso"
		if got != want {
			t.Errorf("getCloudInitVolumeName() = %q, want %q", got, want)
		}
	})
}

// TestCreateFromConfigWithDeps_DataDiskFailure tests data disk creation failure
func TestCreateFromConfigWithDeps_DataDiskFailure(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfigWithDataDisks()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	// Make data disk creation fail
	callCount := 0
	sm.createVolumeFunc = func(ctx context.Context, poolName string, spec storage.VolumeSpec) error {
		callCount++
		if callCount == 2 { // First is boot, second is data disk
			return errors.New("data disk creation failed")
		}
		return nil
	}

	err := createFromConfigWithDeps(ctx, vm, lv, sm)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "data volume") {
		t.Errorf("expected data volume error, got: %v", err)
	}

	// Storage cleanup should happen (boot volume created)
	if len(sm.deleteVolumeCalls) == 0 {
		t.Error("expected storage cleanup")
	}
}

// TestCreateFromConfigWithDeps_CloudInitFailures tests cloud-init related failures
func TestCreateFromConfigWithDeps_CloudInitFailures(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mockStorageManager)
		wantErr   string
	}{
		{
			name: "cloud-init volume creation fails",
			setupMock: func(sm *mockStorageManager) {
				callCount := 0
				sm.createVolumeFunc = func(ctx context.Context, poolName string, spec storage.VolumeSpec) error {
					callCount++
					// Boot volume succeeds, cloud-init fails
					if spec.Type == storage.VolumeTypeCloudInit {
						return errors.New("cloud-init volume creation failed")
					}
					return nil
				}
			},
			wantErr: "failed to create cloud-init volume",
		},
		{
			name: "cloud-init write data fails",
			setupMock: func(sm *mockStorageManager) {
				sm.writeVolumeDataFunc = func(ctx context.Context, poolName, volumeName string, data []byte) error {
					return errors.New("write failed")
				}
			},
			wantErr: "failed to write cloud-init data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfigWithCloudInit()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			tt.setupMock(sm)

			err := createFromConfigWithDeps(ctx, vm, lv, sm)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}

			// Storage cleanup should happen
			if len(sm.deleteVolumeCalls) == 0 {
				t.Error("expected storage cleanup")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_ImagePathHandling tests image path resolution
func TestCreateFromConfigWithDeps_ImagePathHandling(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		setupMock func(*mockStorageManager)
		wantErr   string
	}{
		{
			name:  "file path image",
			image: "/var/lib/libvirt/images/fedora.qcow2",
			setupMock: func(sm *mockStorageManager) {
				// File paths shouldn't trigger image existence checks
			},
			wantErr: "",
		},
		{
			name:  "image check error",
			image: "fedora-43",
			setupMock: func(sm *mockStorageManager) {
				sm.imageExistsFunc = func(ctx context.Context, imageName string) (bool, error) {
					return false, errors.New("image check failed")
				}
			},
			wantErr: "failed to check if image exists",
		},
		{
			name:  "get image path fails",
			image: "fedora-43",
			setupMock: func(sm *mockStorageManager) {
				sm.getImagePathFunc = func(ctx context.Context, imageName string) (string, error) {
					return "", errors.New("path lookup failed")
				}
			},
			wantErr: "failed to get image path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfig()
			vm.Spec.BootDisk.Image = tt.image
			vm.Spec.BootDisk.Empty = false

			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			tt.setupMock(sm)

			err := createFromConfigWithDeps(ctx, vm, lv, sm)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

// TestCreateFromConfigWithDeps_VolumeExistsCheckError tests error during volume exists check
func TestCreateFromConfigWithDeps_VolumeExistsCheckError(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfig()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	sm.volumeExistsFunc = func(ctx context.Context, poolName, volumeName string) (bool, error) {
		return false, errors.New("volume check failed")
	}

	err := createFromConfigWithDeps(ctx, vm, lv, sm)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to check boot volume") {
		t.Errorf("expected volume check error, got: %v", err)
	}
}

// TestCleanupWithDeps_NilDependencies tests cleanup with nil dependencies
func TestCleanupWithDeps_NilDependencies(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfig()

	// Should not panic with nil dependencies
	cleanupWithDeps(ctx, vm, nil, nil, false, false)
	cleanupWithDeps(ctx, vm, nil, nil, true, true)
}

// TestCleanupWithDeps_WithDataDisks tests cleanup with multiple data disks
func TestCleanupWithDeps_WithDataDisks(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfigWithDataDisks()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	cleanupWithDeps(ctx, vm, sm, lv, false, true)

	// Should attempt to delete boot + 2 data disks = 3 volumes
	if len(sm.deleteVolumeCalls) != 3 {
		t.Errorf("expected 3 volume deletes (boot + 2 data), got %d", len(sm.deleteVolumeCalls))
	}
}

// TestCleanupWithDeps_WithCloudInit tests cleanup with cloud-init
func TestCleanupWithDeps_WithCloudInit(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfigWithCloudInit()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	cleanupWithDeps(ctx, vm, sm, lv, false, true)

	// Should attempt to delete boot + cloud-init = 2 volumes
	if len(sm.deleteVolumeCalls) != 2 {
		t.Errorf("expected 2 volume deletes (boot + cloud-init), got %d", len(sm.deleteVolumeCalls))
	}

	// Verify cloud-init volume name
	foundCloudInit := false
	for _, vol := range sm.deleteVolumeCalls {
		if strings.Contains(vol, "cloudinit") {
			foundCloudInit = true
		}
	}
	if !foundCloudInit {
		t.Error("expected cloud-init volume to be deleted")
	}
}

// TestCreateFromConfigWithDeps_MetadataStoreFailure tests metadata storage failure (non-fatal)
func TestCreateFromConfigWithDeps_MetadataStoreFailure(t *testing.T) {
	ctx := context.Background()
	vm := testVMConfig()
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	// Make metadata storage fail
	lv.domainSetMetadataFunc = func(dom libvirt.Domain, typ int32, metadata libvirt.OptString, key libvirt.OptString, uri libvirt.OptString, flags libvirt.DomainModificationImpact) error {
		return errors.New("metadata storage failed")
	}

	// Should succeed despite metadata failure (it's just a warning)
	err := createFromConfigWithDeps(ctx, vm, lv, sm)

	if err != nil {
		t.Fatalf("expected success (metadata failure is non-fatal), got error: %v", err)
	}

	// Verify domain was created
	if len(lv.domainCreateCalls) != 1 {
		t.Error("expected domain to be created despite metadata failure")
	}
}

// TestCreateFromConfigWithDeps_AutostartVariations tests autostart handling
func TestCreateFromConfigWithDeps_AutostartVariations(t *testing.T) {
	tests := []struct {
		name              string
		autostart         *bool
		expectedAutostart int32
	}{
		{
			name:              "autostart nil (default true)",
			autostart:         nil,
			expectedAutostart: 1,
		},
		{
			name: "autostart explicitly true",
			autostart: func() *bool {
				b := true
				return &b
			}(),
			expectedAutostart: 1,
		},
		{
			name: "autostart explicitly false",
			autostart: func() *bool {
				b := false
				return &b
			}(),
			expectedAutostart: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			vm := testVMConfig()
			vm.Spec.Autostart = tt.autostart

			lv := newMockLibvirtClient()
			sm := newMockStorageManager()

			// Track autostart value
			var actualAutostart int32
			lv.domainSetAutostartFunc = func(dom libvirt.Domain, autostart int32) error {
				actualAutostart = autostart
				return nil
			}

			err := createFromConfigWithDeps(ctx, vm, lv, sm)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if actualAutostart != tt.expectedAutostart {
				t.Errorf("expected autostart=%d, got %d", tt.expectedAutostart, actualAutostart)
			}
		})
	}
}
