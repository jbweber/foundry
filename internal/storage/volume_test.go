package storage

import (
	"context"
	"testing"
)

func TestManager_CreateVolume(t *testing.T) {
	tests := []struct {
		name     string
		poolName string
		spec     VolumeSpec
		setup    func(*mockLibvirtClient, *Manager)
		wantErr  bool
	}{
		{
			name:     "create boot disk",
			poolName: "test-pool",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "create boot disk with backing volume",
			poolName: "test-pool",
			spec: VolumeSpec{
				Name:          "my-vm_boot",
				Type:          VolumeTypeBoot,
				Format:        VolumeFormatQCOW2,
				CapacityGB:    50,
				BackingVolume: "fedora-43",
			},
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
				// Create backing volume first
				_ = mgr.CreateVolume(context.Background(), "test-pool", VolumeSpec{
					Name:       "fedora-43",
					Type:       VolumeTypeBaseImage,
					Format:     VolumeFormatQCOW2,
					CapacityGB: 10,
				})
			},
			wantErr: false,
		},
		{
			name:     "create data disk",
			poolName: "test-pool",
			spec: VolumeSpec{
				Name:       "my-vm_data-vdb",
				Type:       VolumeTypeData,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 100,
			},
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "create cloud-init ISO",
			poolName: "test-pool",
			spec: VolumeSpec{
				Name:   "my-vm_cloudinit",
				Type:   VolumeTypeCloudInit,
				Format: VolumeFormatRaw,
			},
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: false,
		},
		{
			name:     "invalid volume spec",
			poolName: "test-pool",
			spec: VolumeSpec{
				Name:       "",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: true,
		},
		{
			name:     "pool not found",
			poolName: "nonexistent",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			setup:   func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.CreateVolume(context.Background(), tt.poolName, tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateVolume() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_DeleteVolume(t *testing.T) {
	tests := []struct {
		name       string
		poolName   string
		volumeName string
		setup      func(*mockLibvirtClient, *Manager)
		wantErr    bool
	}{
		{
			name:       "delete existing volume",
			poolName:   "test-pool",
			volumeName: "test-vol",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
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
			name:       "delete non-existent volume",
			poolName:   "test-pool",
			volumeName: "nonexistent",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: true,
		},
		{
			name:       "pool not found",
			poolName:   "nonexistent",
			volumeName: "test-vol",
			setup:      func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.DeleteVolume(context.Background(), tt.poolName, tt.volumeName)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteVolume() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_ListVolumes(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create a pool
	_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")

	// Create some volumes
	_ = mgr.CreateVolume(context.Background(), "test-pool", VolumeSpec{
		Name:       "vol1",
		Type:       VolumeTypeBoot,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 50,
	})
	_ = mgr.CreateVolume(context.Background(), "test-pool", VolumeSpec{
		Name:       "vol2",
		Type:       VolumeTypeData,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 100,
	})

	volumes, err := mgr.ListVolumes(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("ListVolumes() error = %v", err)
	}

	if len(volumes) != 2 {
		t.Errorf("ListVolumes() returned %d volumes, want 2", len(volumes))
	}
}

func TestManager_GetVolumePath(t *testing.T) {
	tests := []struct {
		name       string
		poolName   string
		volumeName string
		setup      func(*mockLibvirtClient, *Manager)
		wantErr    bool
	}{
		{
			name:       "get path for existing volume",
			poolName:   "test-pool",
			volumeName: "test-vol",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
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
			name:       "pool not found",
			poolName:   "nonexistent",
			volumeName: "test-vol",
			setup:      func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:    true,
		},
		{
			name:       "volume not found",
			poolName:   "test-pool",
			volumeName: "nonexistent",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			path, err := mgr.GetVolumePath(context.Background(), tt.poolName, tt.volumeName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetVolumePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && path == "" {
				t.Errorf("GetVolumePath() returned empty path")
			}
		})
	}
}

func TestManager_WriteVolumeData(t *testing.T) {
	tests := []struct {
		name       string
		poolName   string
		volumeName string
		data       []byte
		setup      func(*mockLibvirtClient, *Manager)
		wantErr    bool
	}{
		{
			name:       "write data to existing volume",
			poolName:   "test-pool",
			volumeName: "test-vol",
			data:       []byte("test data"),
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
				_ = mgr.CreateVolume(context.Background(), "test-pool", VolumeSpec{
					Name:   "test-vol",
					Type:   VolumeTypeCloudInit,
					Format: VolumeFormatRaw,
				})
			},
			wantErr: false,
		},
		{
			name:       "pool not found",
			poolName:   "nonexistent",
			volumeName: "test-vol",
			data:       []byte("test data"),
			setup:      func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:    true,
		},
		{
			name:       "volume not found",
			poolName:   "test-pool",
			volumeName: "nonexistent",
			data:       []byte("test data"),
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), "test-pool", PoolTypeDir, "/var/lib/libvirt/images/test")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.WriteVolumeData(context.Background(), tt.poolName, tt.volumeName, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("WriteVolumeData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify data was written (check in mock)
				vol := mockClient.volumes[tt.poolName][tt.volumeName]
				if string(vol.data) != string(tt.data) {
					t.Errorf("WriteVolumeData() data = %v, want %v", string(vol.data), string(tt.data))
				}
			}
		})
	}
}

func TestManager_VolumeExists(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create a pool and volume
	poolName := "test-pool"
	volumeName := "test-vol"
	_ = mgr.CreatePool(context.Background(), poolName, PoolTypeDir, "/var/lib/libvirt/images/test")
	_ = mgr.CreateVolume(context.Background(), poolName, VolumeSpec{
		Name:       volumeName,
		Type:       VolumeTypeBoot,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 50,
	})

	// Check existing volume
	exists, err := mgr.VolumeExists(context.Background(), poolName, volumeName)
	if err != nil {
		t.Fatalf("VolumeExists() error = %v", err)
	}
	if !exists {
		t.Errorf("VolumeExists() = false, want true")
	}

	// Check non-existent volume
	exists, err = mgr.VolumeExists(context.Background(), poolName, "nonexistent")
	if err != nil {
		t.Fatalf("VolumeExists() error = %v", err)
	}
	if exists {
		t.Errorf("VolumeExists() = true, want false")
	}
}
