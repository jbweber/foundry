package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/digitalocean/go-libvirt"
)

// LibvirtClient is the interface for libvirt operations.
// This allows for dependency injection and testing.
type LibvirtClient interface {
	StoragePoolLookupByName(Name string) (libvirt.StoragePool, error)
	StoragePoolDefineXML(XML string, Flags uint32) (libvirt.StoragePool, error)
	StoragePoolCreate(Pool libvirt.StoragePool, Flags libvirt.StoragePoolCreateFlags) error
	StoragePoolBuild(Pool libvirt.StoragePool, Flags libvirt.StoragePoolBuildFlags) error
	StoragePoolSetAutostart(Pool libvirt.StoragePool, Autostart int32) error
	StoragePoolDestroy(Pool libvirt.StoragePool) error
	StoragePoolUndefine(Pool libvirt.StoragePool) error
	StoragePoolGetInfo(Pool libvirt.StoragePool) (rState uint8, rCapacity uint64, rAllocation uint64, rAvailable uint64, err error)
	StoragePoolGetXMLDesc(Pool libvirt.StoragePool, Flags libvirt.StorageXMLFlags) (string, error)
	StoragePoolListAllVolumes(Pool libvirt.StoragePool, NeedResults int32, Flags uint32) ([]libvirt.StorageVol, uint32, error)
	StoragePoolRefresh(Pool libvirt.StoragePool, Flags uint32) error
	StorageVolLookupByName(Pool libvirt.StoragePool, Name string) (libvirt.StorageVol, error)
	StorageVolCreateXML(Pool libvirt.StoragePool, XML string, Flags libvirt.StorageVolCreateFlags) (libvirt.StorageVol, error)
	StorageVolDelete(Vol libvirt.StorageVol, Flags libvirt.StorageVolDeleteFlags) error
	StorageVolGetPath(Vol libvirt.StorageVol) (string, error)
	StorageVolGetInfo(Vol libvirt.StorageVol) (rType int8, rCapacity uint64, rAllocation uint64, err error)
	StorageVolUpload(Vol libvirt.StorageVol, outStream io.Reader, Offset uint64, Length uint64, Flags libvirt.StorageVolUploadFlags) error
	ConnectListAllStoragePools(NeedResults int32, Flags libvirt.ConnectListAllStoragePoolsFlags) ([]libvirt.StoragePool, uint32, error)
}

// Manager coordinates storage operations for pools, volumes, and images.
type Manager struct {
	client LibvirtClient
}

// NewManager creates a new storage manager.
func NewManager(client LibvirtClient) *Manager {
	return &Manager{
		client: client,
	}
}

// EnsureDefaultPools ensures that the default foundry-images and foundry-vms pools exist.
// This is called automatically during VM creation if needed.
func (m *Manager) EnsureDefaultPools(ctx context.Context) error {
	// Ensure foundry-images pool exists
	if err := m.EnsurePool(ctx, DefaultImagesPool, PoolTypeDir, DefaultImagesPath); err != nil {
		return fmt.Errorf("failed to ensure images pool: %w", err)
	}

	// Ensure foundry-vms pool exists
	if err := m.EnsurePool(ctx, DefaultVMsPool, PoolTypeDir, DefaultVMsPath); err != nil {
		return fmt.Errorf("failed to ensure VMs pool: %w", err)
	}

	return nil
}
