package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportImage imports a base image from a local file into the foundry-images pool.
func (m *Manager) ImportImage(ctx context.Context, filePath, imageName string) error {
	// Check that the file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat image file: %w", err)
	}

	// Get file size in GB (rounded up)
	sizeGB := uint64(info.Size()/(1024*1024*1024)) + 1

	// Read the image file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read image file: %w", err)
	}

	// Determine format based on file extension
	format := VolumeFormatQCOW2
	ext := filepath.Ext(filePath)
	if ext == ".raw" || ext == ".img" {
		format = VolumeFormatRaw
	}

	// Ensure imageName has the correct extension matching the format
	expectedExt := ".qcow2"
	if format == VolumeFormatRaw {
		expectedExt = ".raw"
	}
	if !strings.HasSuffix(imageName, expectedExt) {
		// Remove any existing extension
		if currentExt := filepath.Ext(imageName); currentExt != "" {
			imageName = strings.TrimSuffix(imageName, currentExt)
		}
		// Add correct extension
		imageName = imageName + expectedExt
	}

	// Create a volume in the foundry-images pool
	spec := VolumeSpec{
		Name:       imageName,
		Type:       VolumeTypeBaseImage,
		Format:     format,
		CapacityGB: sizeGB,
	}

	if err := m.CreateVolume(ctx, DefaultImagesPool, spec); err != nil {
		return fmt.Errorf("failed to create image volume: %w", err)
	}

	// Upload the image data to the volume
	if err := m.WriteVolumeData(ctx, DefaultImagesPool, imageName, data); err != nil {
		// Clean up the volume if upload fails
		_ = m.DeleteVolume(ctx, DefaultImagesPool, imageName)
		return fmt.Errorf("failed to upload image data: %w", err)
	}

	return nil
}

// PullImage downloads and imports a base image from a URL.
// This is a placeholder for future implementation.
func (m *Manager) PullImage(ctx context.Context, url, imageName, checksum string) error {
	return fmt.Errorf("pull image is not yet implemented")
}

// ListImages lists all base images in the foundry-images pool.
func (m *Manager) ListImages(ctx context.Context) ([]VolumeInfo, error) {
	return m.ListVolumes(ctx, DefaultImagesPool)
}

// DeleteImage deletes a base image from the foundry-images pool.
// If force is true, the image is deleted even if it's being used as a backing file.
func (m *Manager) DeleteImage(ctx context.Context, imageName string, force bool) error {
	// TODO: If force is false, check if the image is being used as a backing file
	// This would require iterating through all VMs and checking their backing stores
	// For now, we just delete the image regardless of force flag

	// Note: force parameter is reserved for future use when we implement backing file checks
	_ = force

	return m.DeleteVolume(ctx, DefaultImagesPool, imageName)
}

// GetImagePath gets the full filesystem path for a base image.
func (m *Manager) GetImagePath(ctx context.Context, imageName string) (string, error) {
	return m.GetVolumePath(ctx, DefaultImagesPool, imageName)
}

// ImageExists checks if a base image exists in the foundry-images pool.
func (m *Manager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	return m.VolumeExists(ctx, DefaultImagesPool, imageName)
}
