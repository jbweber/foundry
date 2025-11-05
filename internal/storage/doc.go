// Package storage provides libvirt storage pool and volume management.
//
// This package handles all storage operations including:
//   - Pool lifecycle (create, delete, refresh, list)
//   - Volume operations (create, delete, clone, list)
//   - Image management (import, list, delete, validation)
//   - Format detection and validation (QCOW2, RAW)
//
// Storage Architecture:
//
// Foundry uses a pool-based storage architecture with two default pools:
//   - foundry-images: Base OS images shared across VMs
//   - foundry-vms: VM-specific volumes (boot disks, data disks, cloud-init ISOs)
//
// Volume Naming Convention:
//
// Volumes follow a predictable naming pattern (see internal/naming package):
//   - Boot disk: {vm-name}_boot
//   - Data disk: {vm-name}_data-{device}
//   - Cloud-init: {vm-name}_cloudinit
//
// Format Validation:
//
// The package performs pure Go magic byte detection to validate image formats:
//   - QCOW2: Magic bytes "QFI\xfb" at offset 0
//   - RAW: MBR signature 0x55aa at offset 510
//   - Rejects format mismatches (e.g., RAW file with .qcow2 extension)
//
// Consumer-Side Interface:
//
// The LibvirtClient interface is defined by consumers (e.g., internal/vm)
// to request only the libvirt operations they need. This package's Manager
// type satisfies those interfaces implicitly. See storage.LibvirtClient for
// the interface used by this package for its own libvirt operations.
//
// Example usage:
//
//	// Connect to libvirt
//	client, err := libvirt.Connect()
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Create storage manager
//	mgr := storage.NewManager(client.Libvirt())
//
//	// Ensure default pools exist
//	if err := mgr.EnsureDefaultPools(ctx); err != nil {
//	    return err
//	}
//
//	// Import an image
//	if err := mgr.ImportImage(ctx, "/path/to/image.qcow2", "fedora-43.qcow2"); err != nil {
//	    return err
//	}
//
//	// Create a boot volume from the image
//	spec := storage.VolumeSpec{
//	    Name:       "myvm_boot",
//	    Type:       storage.VolumeTypeBoot,
//	    Format:     storage.VolumeFormatQCOW2,
//	    CapacityGB: 20,
//	    BackingVolume: &storage.BackingVolume{
//	        Pool:   "foundry-images",
//	        Volume: "fedora-43.qcow2",
//	    },
//	}
//	if err := mgr.CreateVolume(ctx, "foundry-vms", spec); err != nil {
//	    return err
//	}
package storage
