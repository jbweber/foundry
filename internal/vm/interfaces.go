package vm

import (
	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/internal/config"
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

	// DomainDestroy force-stops a domain
	DomainDestroy(dom libvirt.Domain) error

	// DomainUndefine undefines a domain
	DomainUndefine(dom libvirt.Domain) error
}

// storageManager defines the storage operations needed for VM management.
// This allows for dependency injection and testing.
//
// In production, this is satisfied by *disk.Manager.
// In tests, this is satisfied by mock implementations.
type storageManager interface {
	// CheckDiskSpace verifies that sufficient disk space is available
	CheckDiskSpace(cfg *config.VMConfig) error

	// VMDirectoryExists checks if the VM directory already exists
	VMDirectoryExists(vmName string) (bool, error)

	// GetVMDirectory returns the full path to the VM's storage directory
	GetVMDirectory(vmName string) string

	// CreateVMDirectory creates the VM storage directory with proper permissions
	CreateVMDirectory(vmName string) error

	// CreateBootDisk creates a boot disk using qemu-img
	CreateBootDisk(cfg *config.VMConfig) error

	// CreateDataDisk creates a data disk using qemu-img
	CreateDataDisk(vmName string, disk config.DataDiskConfig) error

	// WriteCloudInitISO writes the cloud-init ISO to disk
	WriteCloudInitISO(cfg *config.VMConfig, isoData []byte) error

	// DeleteVM removes the entire VM directory and all its contents
	DeleteVM(vmName string) error
}
