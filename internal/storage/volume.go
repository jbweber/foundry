package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	libvirtxml "libvirt.org/go/libvirtxml"
)

// CreateVolume creates a new volume in the specified pool.
func (m *Manager) CreateVolume(ctx context.Context, poolName string, spec VolumeSpec) error {
	// Validate the volume spec
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid volume spec: %w", err)
	}

	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	// Generate volume XML
	volumeXML, err := generateVolumeXML(poolName, spec, m)
	if err != nil {
		return fmt.Errorf("failed to generate volume XML: %w", err)
	}

	// Create the volume
	_, err = m.client.StorageVolCreateXML(pool, volumeXML, 0)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	return nil
}

// DeleteVolume deletes a volume from the specified pool.
func (m *Manager) DeleteVolume(ctx context.Context, poolName, volumeName string) error {
	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	// Look up the volume
	vol, err := m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		return fmt.Errorf("volume not found: %w", err)
	}

	// Delete the volume
	if err := m.client.StorageVolDelete(vol, 0); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

// ListVolumes lists all volumes in the specified pool.
func (m *Manager) ListVolumes(ctx context.Context, poolName string) ([]VolumeInfo, error) {
	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("pool not found: %w", err)
	}

	// List volumes
	volumes, _, err := m.client.StoragePoolListAllVolumes(pool, 1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	var volumeInfos []VolumeInfo
	for _, vol := range volumes {
		// Get volume path
		path, err := m.client.StorageVolGetPath(vol)
		if err != nil {
			// Skip volumes we can't get the path for
			continue
		}

		// Get volume info
		_, capacity, allocation, err := m.client.StorageVolGetInfo(vol)
		if err != nil {
			// Skip volumes we can't get info for
			continue
		}

		volumeInfos = append(volumeInfos, VolumeInfo{
			Name:       vol.Name,
			Path:       path,
			Pool:       poolName,
			Capacity:   capacity,
			Allocation: allocation,
			// Type and Format would require parsing XML, skip for now
		})
	}

	return volumeInfos, nil
}

// GetVolumePath gets the full filesystem path for a volume.
func (m *Manager) GetVolumePath(ctx context.Context, poolName, volumeName string) (string, error) {
	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return "", fmt.Errorf("pool not found: %w", err)
	}

	// Look up the volume
	vol, err := m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		return "", fmt.Errorf("volume not found: %w", err)
	}

	// Get volume path
	path, err := m.client.StorageVolGetPath(vol)
	if err != nil {
		return "", fmt.Errorf("failed to get volume path: %w", err)
	}

	return path, nil
}

// WriteVolumeData uploads data to a volume (used for cloud-init ISOs).
func (m *Manager) WriteVolumeData(ctx context.Context, poolName, volumeName string, data []byte) error {
	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	// Look up the volume
	vol, err := m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		return fmt.Errorf("volume not found: %w", err)
	}

	// Upload data to volume
	reader := bytes.NewReader(data)
	if err := m.client.StorageVolUpload(vol, reader, 0, uint64(len(data)), 0); err != nil {
		return fmt.Errorf("failed to upload data to volume: %w", err)
	}

	return nil
}

// VolumeExists checks if a volume exists in the specified pool.
func (m *Manager) VolumeExists(ctx context.Context, poolName, volumeName string) (bool, error) {
	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return false, fmt.Errorf("pool not found: %w", err)
	}

	// Try to look up the volume
	_, err = m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		// Volume doesn't exist
		return false, nil
	}

	return true, nil
}

// generateVolumeXML generates XML for a storage volume.
func generateVolumeXML(poolName string, spec VolumeSpec, m *Manager) (string, error) {
	// Convert capacity from GB to bytes
	capacityBytes := spec.CapacityGB * 1024 * 1024 * 1024

	vol := &libvirtxml.StorageVolume{
		Type: "file",
		Name: spec.Name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: capacityBytes,
			Unit:  "B",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: string(spec.Format),
			},
			Permissions: &libvirtxml.StorageVolumeTargetPermissions{
				Owner: "107", // qemu user
				Group: "107", // qemu group
				Mode:  "0644",
			},
		},
	}

	// Add backing store if specified
	if spec.BackingVolume != "" {
		// Get the backing volume path
		backingPath, err := m.GetVolumePath(context.Background(), poolName, spec.BackingVolume)
		if err != nil {
			return "", fmt.Errorf("failed to get backing volume path: %w", err)
		}

		vol.BackingStore = &libvirtxml.StorageVolumeBackingStore{
			Path: backingPath,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: string(spec.Format),
			},
		}
	}

	xmlBytes, err := vol.Marshal()
	if err != nil {
		return "", err
	}

	// Clean up the XML: remove standalone attribute
	xml := string(xmlBytes)
	xml = strings.TrimPrefix(xml, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
	xml = strings.TrimSpace(xml)

	return xml, nil
}
