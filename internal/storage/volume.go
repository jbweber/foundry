package storage

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	libvirtxml "libvirt.org/go/libvirtxml"
)

// CreateVolume creates a new volume in the specified pool.
func (m *Manager) CreateVolume(_ context.Context, poolName string, spec VolumeSpec) error {
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
func (m *Manager) DeleteVolume(_ context.Context, poolName, volumeName string) error {
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
func (m *Manager) ListVolumes(_ context.Context, poolName string) ([]VolumeInfo, error) {
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
func (m *Manager) GetVolumePath(_ context.Context, poolName, volumeName string) (string, error) {
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
func (m *Manager) WriteVolumeData(_ context.Context, poolName, volumeName string, data []byte) error {
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
func (m *Manager) VolumeExists(_ context.Context, poolName, volumeName string) (bool, error) {
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
func generateVolumeXML(_ string, spec VolumeSpec, _ *Manager) (string, error) {
	// Convert capacity from GB to bytes
	capacityBytes := spec.CapacityGB * 1024 * 1024 * 1024

	// Get the QEMU user/group IDs for this system
	uid, gid, _ := GetQEMUUserGroup()

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
				Owner: uid,
				Group: gid,
				Mode:  "0644",
			},
		},
	}

	// Add backing store if specified
	if spec.BackingVolume != "" {
		// BackingVolume should be a filesystem path (not pool:volume reference).
		// This is necessary because backing images are typically in a different pool
		// (e.g., foundry-images) than the volume being created (e.g., foundry-vms).
		// Libvirt's XML schema requires a filesystem path in the backing store element.
		// The caller is responsible for resolving pool:volume references to paths.
		//
		// Determine the backing file format from the file extension.
		// We enforce that all image files have .qcow2 or .raw extensions, so we can
		// reliably determine the format from the path.
		backingFormat := "qcow2" // default
		ext := filepath.Ext(spec.BackingVolume)
		if ext == ".raw" || ext == ".img" {
			backingFormat = "raw"
		}

		vol.BackingStore = &libvirtxml.StorageVolumeBackingStore{
			Path: spec.BackingVolume,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: backingFormat,
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
