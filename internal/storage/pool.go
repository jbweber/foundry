package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/digitalocean/go-libvirt"
	libvirtxml "libvirt.org/go/libvirtxml"
)

// EnsurePool ensures a storage pool exists, creating it if necessary.
// If the pool already exists, this is a no-op.
func (m *Manager) EnsurePool(ctx context.Context, name string, poolType PoolType, path string) error {
	// Check if pool already exists
	_, err := m.client.StoragePoolLookupByName(name)
	if err == nil {
		// Pool exists, nothing to do
		return nil
	}

	// Pool doesn't exist, create it
	return m.CreatePool(ctx, name, poolType, path)
}

// CreatePool creates a new storage pool.
// Returns an error if the pool already exists.
func (m *Manager) CreatePool(ctx context.Context, name string, poolType PoolType, path string) error {
	// Generate pool XML based on type
	var poolXML string
	var err error

	switch poolType {
	case PoolTypeDir:
		poolXML, err = generateDirPoolXML(name, path)
	default:
		return fmt.Errorf("unsupported pool type: %s", poolType)
	}

	if err != nil {
		return fmt.Errorf("failed to generate pool XML: %w", err)
	}

	// Define the pool
	pool, err := m.client.StoragePoolDefineXML(poolXML, 0)
	if err != nil {
		return fmt.Errorf("failed to define pool: %w", err)
	}

	// Build the pool (creates the directory structure)
	if err := m.client.StoragePoolBuild(pool, 0); err != nil {
		// Try to undefine the pool if build fails
		_ = m.client.StoragePoolUndefine(pool)
		return fmt.Errorf("failed to build pool: %w", err)
	}

	// Start the pool
	if err := m.client.StoragePoolCreate(pool, 0); err != nil {
		// Try to undefine the pool if start fails
		_ = m.client.StoragePoolUndefine(pool)
		return fmt.Errorf("failed to start pool: %w", err)
	}

	// Set autostart
	if err := m.client.StoragePoolSetAutostart(pool, 1); err != nil {
		// Pool is created and started, but autostart failed
		// This is not critical, so we just return the error
		return fmt.Errorf("pool created but failed to set autostart: %w", err)
	}

	return nil
}

// DeletePool deletes a storage pool.
// If force is true, all volumes in the pool are deleted first.
// Returns an error if the pool doesn't exist or if deletion fails.
func (m *Manager) DeletePool(ctx context.Context, name string, force bool) error {
	// Prevent deletion of default pools
	if name == DefaultImagesPool || name == DefaultVMsPool {
		return fmt.Errorf("cannot delete default pool: %s", name)
	}

	// Look up the pool
	pool, err := m.client.StoragePoolLookupByName(name)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	// If force is true, delete all volumes first
	if force {
		volumes, _, err := m.client.StoragePoolListAllVolumes(pool, 1, 0)
		if err != nil {
			return fmt.Errorf("failed to list volumes: %w", err)
		}

		for _, vol := range volumes {
			if err := m.client.StorageVolDelete(vol, 0); err != nil {
				// Continue deleting other volumes even if one fails
				continue
			}
		}
	}

	// Stop the pool if it's running
	poolState, _, _, _, err := m.client.StoragePoolGetInfo(pool)
	if err != nil {
		return fmt.Errorf("failed to get pool info: %w", err)
	}

	if libvirt.StoragePoolState(poolState) == libvirt.StoragePoolRunning {
		if err := m.client.StoragePoolDestroy(pool); err != nil {
			return fmt.Errorf("failed to stop pool: %w", err)
		}
	}

	// Undefine the pool
	if err := m.client.StoragePoolUndefine(pool); err != nil {
		return fmt.Errorf("failed to undefine pool: %w", err)
	}

	return nil
}

// ListPools lists all storage pools.
func (m *Manager) ListPools(ctx context.Context) ([]PoolInfo, error) {
	pools, _, err := m.client.ConnectListAllStoragePools(1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %w", err)
	}

	var poolInfos []PoolInfo
	for _, pool := range pools {
		info, err := m.GetPoolInfo(ctx, pool.Name)
		if err != nil {
			// Skip pools we can't get info for
			continue
		}
		poolInfos = append(poolInfos, *info)
	}

	return poolInfos, nil
}

// GetPoolInfo gets detailed information about a storage pool.
func (m *Manager) GetPoolInfo(ctx context.Context, name string) (*PoolInfo, error) {
	pool, err := m.client.StoragePoolLookupByName(name)
	if err != nil {
		return nil, fmt.Errorf("pool not found: %w", err)
	}

	// Get pool info (capacity, allocation, etc.)
	poolState, capacity, allocation, available, err := m.client.StoragePoolGetInfo(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool info: %w", err)
	}

	// Get pool XML to extract type and path
	xmlDesc, err := m.client.StoragePoolGetXMLDesc(pool, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool XML: %w", err)
	}

	// Parse XML to get pool details
	var poolDef libvirtxml.StoragePool
	if err := poolDef.Unmarshal(xmlDesc); err != nil {
		return nil, fmt.Errorf("failed to parse pool XML: %w", err)
	}

	// Extract pool type and path
	poolType := PoolTypeDir // Default to dir
	poolPath := ""

	if poolDef.Type == "dir" && poolDef.Target != nil {
		poolType = PoolTypeDir
		poolPath = poolDef.Target.Path
	}

	// Map libvirt state to string
	stateStr := "unknown"
	switch libvirt.StoragePoolState(poolState) {
	case libvirt.StoragePoolInactive:
		stateStr = "inactive"
	case libvirt.StoragePoolBuilding:
		stateStr = "building"
	case libvirt.StoragePoolRunning:
		stateStr = "running"
	case libvirt.StoragePoolDegraded:
		stateStr = "degraded"
	case libvirt.StoragePoolInaccessible:
		stateStr = "inaccessible"
	}

	// Format UUID as string (8-4-4-4-12 hex format)
	uuid := fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		pool.UUID[0], pool.UUID[1], pool.UUID[2], pool.UUID[3],
		pool.UUID[4], pool.UUID[5],
		pool.UUID[6], pool.UUID[7],
		pool.UUID[8], pool.UUID[9],
		pool.UUID[10], pool.UUID[11], pool.UUID[12], pool.UUID[13], pool.UUID[14], pool.UUID[15])

	return &PoolInfo{
		Name:       pool.Name,
		Type:       poolType,
		Path:       poolPath,
		UUID:       uuid,
		State:      stateStr,
		Capacity:   capacity,
		Allocation: allocation,
		Available:  available,
	}, nil
}

// RefreshPool refreshes a storage pool, updating its state.
func (m *Manager) RefreshPool(ctx context.Context, name string) error {
	pool, err := m.client.StoragePoolLookupByName(name)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	if err := m.client.StoragePoolRefresh(pool, 0); err != nil {
		return fmt.Errorf("failed to refresh pool: %w", err)
	}

	return nil
}

// generateDirPoolXML generates XML for a directory-based storage pool.
func generateDirPoolXML(name, path string) (string, error) {
	pool := &libvirtxml.StoragePool{
		Type: "dir",
		Name: name,
		Target: &libvirtxml.StoragePoolTarget{
			Path: path,
			Permissions: &libvirtxml.StoragePoolTargetPermissions{
				Owner: "107", // qemu user (typically uid 107)
				Group: "107", // qemu group (typically gid 107)
				Mode:  "0755",
			},
		},
	}

	xmlBytes, err := pool.Marshal()
	if err != nil {
		return "", err
	}

	// Clean up the XML: remove standalone attribute
	xml := string(xmlBytes)
	xml = strings.TrimPrefix(xml, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
	xml = strings.TrimSpace(xml)

	return xml, nil
}
