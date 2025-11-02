package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_ImportImage(t *testing.T) {
	// Create a temporary image file for testing
	tmpDir := t.TempDir()
	imagePath := filepath.Join(tmpDir, "test-image.qcow2")
	testData := []byte("fake qcow2 data")
	if err := os.WriteFile(imagePath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	tests := []struct {
		name      string
		filePath  string
		imageName string
		setup     func(*mockLibvirtClient, *Manager)
		wantErr   bool
	}{
		{
			name:      "import qcow2 image",
			filePath:  imagePath,
			imageName: "fedora-43",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: false,
		},
		{
			name:      "import non-existent file",
			filePath:  "/nonexistent/image.qcow2",
			imageName: "fedora-43",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
		{
			name:      "pool not found",
			filePath:  imagePath,
			imageName: "fedora-43",
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
			}

			// If successful, verify the image was created
			if err == nil {
				exists, _ := mgr.ImageExists(context.Background(), tt.imageName)
				if !exists {
					t.Errorf("Image %s not found after ImportImage()", tt.imageName)
				}

				// Verify data was uploaded
				vol := mockClient.volumes[DefaultImagesPool][tt.imageName]
				if string(vol.data) != string(testData) {
					t.Errorf("Image data mismatch: got %v, want %v", string(vol.data), string(testData))
				}
			}
		})
	}
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
