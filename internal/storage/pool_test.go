package storage

import (
	"context"
	"testing"
)

func TestManager_EnsurePool(t *testing.T) {
	tests := []struct {
		name      string
		poolName  string
		poolType  PoolType
		path      string
		setup     func(*mockLibvirtClient)
		wantErr   bool
		checkPool bool
	}{
		{
			name:      "create new pool",
			poolName:  "test-pool",
			poolType:  PoolTypeDir,
			path:      "/var/lib/libvirt/images/test",
			setup:     func(m *mockLibvirtClient) {},
			wantErr:   false,
			checkPool: true,
		},
		{
			name:     "pool already exists",
			poolName: "existing-pool",
			poolType: PoolTypeDir,
			path:     "/var/lib/libvirt/images/existing",
			setup: func(m *mockLibvirtClient) {
				// Pre-create the pool
				mgr := NewManager(m)
				_ = mgr.CreatePool(context.Background(), "existing-pool", PoolTypeDir, "/var/lib/libvirt/images/existing")
			},
			wantErr:   false,
			checkPool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			tt.setup(mockClient)

			mgr := NewManager(mockClient)
			err := mgr.EnsurePool(context.Background(), tt.poolName, tt.poolType, tt.path)

			if (err != nil) != tt.wantErr {
				t.Errorf("EnsurePool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkPool {
				// Verify pool exists
				_, err := mockClient.StoragePoolLookupByName(tt.poolName)
				if err != nil {
					t.Errorf("Pool %s not found after EnsurePool()", tt.poolName)
				}
			}
		})
	}
}

func TestManager_CreatePool(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
		poolType PoolType
		path     string
		setup    func(*mockLibvirtClient)
		wantErr  bool
	}{
		{
			name:     "create dir pool",
			poolName: "test-pool",
			poolType: PoolTypeDir,
			path:     "/var/lib/libvirt/images/test",
			setup:    func(m *mockLibvirtClient) {},
			wantErr:  false,
		},
		{
			name:     "unsupported pool type",
			poolName: "lvm-pool",
			poolType: PoolTypeLVM,
			path:     "/dev/vg0",
			setup:    func(m *mockLibvirtClient) {},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			tt.setup(mockClient)

			mgr := NewManager(mockClient)
			err := mgr.CreatePool(context.Background(), tt.poolName, tt.poolType, tt.path)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreatePool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_DeletePool(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
		force    bool
		setup    func(*mockLibvirtClient)
		wantErr  bool
	}{
		{
			name:     "delete empty pool",
			poolName: "test-pool",
			force:    false,
			setup: func(m *mockLibvirtClient) {
				mgr := NewManager(m)
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "delete pool with volumes (force)",
			poolName: "test-pool",
			force:    true,
			setup: func(m *mockLibvirtClient) {
				mgr := NewManager(m)
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
				_ = mgr.CreateVolume(context.Background(), "test-pool", VolumeSpec{
					Name:       "test-vol",
					Type:       VolumeTypeBoot,
					Format:     VolumeFormatQCOW2,
					CapacityGB: 50,
				})
			},
			wantErr: false,
		},
		{
			name:     "cannot delete default images pool",
			poolName: DefaultImagesPool,
			force:    false,
			setup: func(m *mockLibvirtClient) {
				mgr := NewManager(m)
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
		{
			name:     "cannot delete default vms pool",
			poolName: DefaultVMsPool,
			force:    false,
			setup: func(m *mockLibvirtClient) {
				mgr := NewManager(m)
				_ = mgr.CreatePool(context.Background(), DefaultVMsPool, PoolTypeDir, DefaultVMsPath)
			},
			wantErr: true,
		},
		{
			name:     "delete non-existent pool",
			poolName: "nonexistent",
			force:    false,
			setup:    func(m *mockLibvirtClient) {},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			tt.setup(mockClient)

			mgr := NewManager(mockClient)
			err := mgr.DeletePool(context.Background(), tt.poolName, tt.force)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeletePool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_ListPools(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create some pools
	_ = mgr.CreatePool(context.Background(), "pool1", PoolTypeDir, "/var/lib/libvirt/images/pool1")
	_ = mgr.CreatePool(context.Background(), "pool2", PoolTypeDir, "/var/lib/libvirt/images/pool2")

	pools, err := mgr.ListPools(context.Background())
	if err != nil {
		t.Fatalf("ListPools() error = %v", err)
	}

	if len(pools) != 2 {
		t.Errorf("ListPools() returned %d pools, want 2", len(pools))
	}
}

func TestManager_GetPoolInfo(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
		setup    func(*mockLibvirtClient, *Manager)
		wantErr  bool
	}{
		{
			name:     "get info for existing pool",
			poolName: "test-pool",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "pool not found",
			poolName: "nonexistent",
			setup:    func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			info, err := mgr.GetPoolInfo(context.Background(), tt.poolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPoolInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if info.Name != tt.poolName {
					t.Errorf("GetPoolInfo() name = %v, want %v", info.Name, tt.poolName)
				}

				if info.State != "running" {
					t.Errorf("GetPoolInfo() state = %v, want running", info.State)
				}

				if info.Type != PoolTypeDir {
					t.Errorf("GetPoolInfo() type = %v, want %v", info.Type, PoolTypeDir)
				}
			}
		})
	}
}

func TestManager_RefreshPool(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
		setup    func(*mockLibvirtClient, *Manager)
		wantErr  bool
	}{
		{
			name:     "refresh existing pool",
			poolName: "test-pool",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "pool not found",
			poolName: "nonexistent",
			setup:    func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.RefreshPool(context.Background(), tt.poolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("RefreshPool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_EnsureDefaultPools(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	err := mgr.EnsureDefaultPools(context.Background())
	if err != nil {
		t.Fatalf("EnsureDefaultPools() error = %v", err)
	}

	// Verify both default pools exist
	_, err = mockClient.StoragePoolLookupByName(DefaultImagesPool)
	if err != nil {
		t.Errorf("Default images pool not found after EnsureDefaultPools()")
	}

	_, err = mockClient.StoragePoolLookupByName(DefaultVMsPool)
	if err != nil {
		t.Errorf("Default VMs pool not found after EnsureDefaultPools()")
	}
}
