package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_ImportImage(t *testing.T) {
	tmpDir := t.TempDir()

	// Helper to create QCOW2 test file
	createQCOW2 := func(path string) error {
		data := []byte{0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x03} // QCOW2 magic + version
		data = append(data, make([]byte, 504)...)                      // Pad to 512 bytes
		return os.WriteFile(path, data, 0644)
	}

	// Helper to create bootable RAW test file
	createBootableRAW := func(path string) error {
		data := make([]byte, 512)
		data[510] = 0x55
		data[511] = 0xaa
		return os.WriteFile(path, data, 0644)
	}

	// Create test files
	qcow2Path := filepath.Join(tmpDir, "test-image.qcow2")
	if err := createQCOW2(qcow2Path); err != nil {
		t.Fatalf("Failed to create QCOW2 test file: %v", err)
	}

	rawPath := filepath.Join(tmpDir, "test-image.raw")
	if err := createBootableRAW(rawPath); err != nil {
		t.Fatalf("Failed to create RAW test file: %v", err)
	}

	// Create misnamed file (QCOW2 with .raw extension)
	misnamedPath := filepath.Join(tmpDir, "misnamed.raw")
	if err := createQCOW2(misnamedPath); err != nil {
		t.Fatalf("Failed to create misnamed test file: %v", err)
	}

	// Create non-bootable file
	nonBootablePath := filepath.Join(tmpDir, "non-bootable.raw")
	if err := os.WriteFile(nonBootablePath, make([]byte, 512), 0644); err != nil {
		t.Fatalf("Failed to create non-bootable test file: %v", err)
	}

	tests := []struct {
		name      string
		filePath  string
		imageName string
		setup     func(*mockLibvirtClient, *Manager)
		wantErr   bool
		errMsg    string // Expected error substring
	}{
		{
			name:      "import qcow2 image",
			filePath:  qcow2Path,
			imageName: "fedora-43.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: false,
		},
		{
			name:      "import bootable raw image",
			filePath:  rawPath,
			imageName: "ubuntu-24.04.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: false,
		},
		{
			name:      "reject image name without extension",
			filePath:  qcow2Path,
			imageName: "fedora-43",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "must have .qcow2 or .raw extension",
		},
		{
			name:      "reject image name with wrong extension",
			filePath:  qcow2Path,
			imageName: "fedora-43.img",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "must have .qcow2 or .raw extension",
		},
		{
			name:      "reject format mismatch (qcow2 file, raw name)",
			filePath:  qcow2Path,
			imageName: "fedora-43.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject format mismatch (raw file, qcow2 name)",
			filePath:  rawPath,
			imageName: "ubuntu-24.04.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject misnamed file",
			filePath:  misnamedPath,
			imageName: "misnamed.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject non-bootable raw file",
			filePath:  nonBootablePath,
			imageName: "non-bootable.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "unsupported or invalid image",
		},
		{
			name:      "import non-existent file",
			filePath:  "/nonexistent/image.qcow2",
			imageName: "fedora-43.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
		{
			name:      "pool not found",
			filePath:  qcow2Path,
			imageName: "fedora-43.qcow2",
			setup:     func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.ImportImage(context.Background(), tt.filePath, tt.imageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ImportImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If error expected, verify error message contains expected substring
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !contains(err.Error(), tt.errMsg) {
					t.Errorf("ImportImage() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			// If successful, verify the image was created
			if err == nil {
				exists, _ := mgr.ImageExists(context.Background(), tt.imageName)
				if !exists {
					t.Errorf("Image %s not found after ImportImage()", tt.imageName)
				}

				// Verify data was uploaded
				vol := mockClient.volumes[DefaultImagesPool][tt.imageName]
				if len(vol.data) == 0 {
					t.Errorf("Image data not uploaded")
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestManager_ListImages(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)

	// Create some images
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       "fedora-43",
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       "ubuntu-24.04",
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 8,
	})

	images, err := mgr.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}

	if len(images) != 2 {
		t.Errorf("ListImages() returned %d images, want 2", len(images))
	}
}

func TestManager_DeleteImage(t *testing.T) {
	tests := []struct {
		name      string
		imageName string
		force     bool
		setup     func(*mockLibvirtClient, *Manager)
		wantErr   bool
	}{
		{
			name:      "delete existing image",
			imageName: "test-image",
			force:     false,
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
				_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
					Name:       "test-image",
					Type:       VolumeTypeBaseImage,
					Format:     VolumeFormatQCOW2,
					CapacityGB: 10,
				})
			},
			wantErr: false,
		},
		{
			name:      "delete non-existent image",
			imageName: "nonexistent",
			force:     false,
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.DeleteImage(context.Background(), tt.imageName, tt.force)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_GetImagePath(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool and image
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
	imageName := "fedora-43"
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       imageName,
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})

	path, err := mgr.GetImagePath(context.Background(), imageName)
	if err != nil {
		t.Fatalf("GetImagePath() error = %v", err)
	}

	if path == "" {
		t.Errorf("GetImagePath() returned empty path")
	}
}

func TestManager_ImageExists(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool and image
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
	imageName := "fedora-43"
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       imageName,
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})

	// Check existing image
	exists, err := mgr.ImageExists(context.Background(), imageName)
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Errorf("ImageExists() = false, want true")
	}

	// Check non-existent image
	exists, err = mgr.ImageExists(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if exists {
		t.Errorf("ImageExists() = true, want false")
	}
}

func TestManager_PullImage(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// PullImage is not yet implemented
	err := mgr.PullImage(context.Background(), "http://example.com/image.qcow2", "test-image", "")
	if err == nil {
		t.Errorf("PullImage() should return error for unimplemented feature")
	}
}
