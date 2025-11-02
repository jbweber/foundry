package vm

import (
	"context"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/internal/storage"
)

// libvirtClient defines the libvirt operations needed for VM management.
// This wraps operations from *libvirt.Libvirt to allow for testing.
//
// In production, this is satisfied by *libvirt.Libvirt directly.
// In tests, this is satisfied by mock implementations.
type libvirtClient interface {
	// DomainLookupByName looks up a domain by name
	DomainLookupByName(name string) (libvirt.Domain, error)

	// DomainDefineXML defines a domain from XML
	DomainDefineXML(xml string) (libvirt.Domain, error)

	// DomainSetAutostart sets autostart for a domain
	DomainSetAutostart(dom libvirt.Domain, autostart int32) error

	// DomainCreate starts a domain
	DomainCreate(dom libvirt.Domain) error

	// DomainGetState gets the state of a domain
	DomainGetState(dom libvirt.Domain, flags uint32) (state int32, reason int32, err error)

	// DomainShutdown gracefully shuts down a domain
	DomainShutdown(dom libvirt.Domain) error

	// DomainDestroy force-stops a domain
	DomainDestroy(dom libvirt.Domain) error

	// DomainUndefineFlags undefines a domain with flags (e.g., NVRAM cleanup)
	DomainUndefineFlags(dom libvirt.Domain, flags libvirt.DomainUndefineFlagsValues) error

	// DomainUndefine undefines a domain
	DomainUndefine(dom libvirt.Domain) error
}

// storageManager defines the storage operations needed for VM management.
// This allows for dependency injection and testing.
//
// In production, this is satisfied by *storage.Manager.
// In tests, this is satisfied by mock implementations.
type storageManager interface {
	// EnsureDefaultPools ensures the default foundry-images and foundry-vms pools exist
	EnsureDefaultPools(ctx context.Context) error

	// VolumeExists checks if a volume exists in a pool
	VolumeExists(ctx context.Context, poolName, volumeName string) (bool, error)

	// CreateVolume creates a new volume in a pool
	CreateVolume(ctx context.Context, poolName string, spec storage.VolumeSpec) error

	// DeleteVolume deletes a volume from a pool
	DeleteVolume(ctx context.Context, poolName, volumeName string) error

	// GetImagePath returns the filesystem path to an image volume
	GetImagePath(ctx context.Context, imageName string) (string, error)

	// ImageExists checks if an image exists in the foundry-images pool
	ImageExists(ctx context.Context, imageName string) (bool, error)

	// WriteVolumeData writes data to a volume (for cloud-init ISOs)
	WriteVolumeData(ctx context.Context, poolName, volumeName string, data []byte) error

	// ListVolumes lists all volumes in a pool
	ListVolumes(ctx context.Context, poolName string) ([]storage.VolumeInfo, error)
}
