package storage

import "fmt"

// PoolType represents the type of storage pool backend.
type PoolType string

const (
	PoolTypeDir     PoolType = "dir"     // Directory-based storage
	PoolTypeLVM     PoolType = "lvm"     // LVM volume group
	PoolTypeZFS     PoolType = "zfs"     // ZFS pool
	PoolTypeNFS     PoolType = "netfs"   // NFS mount
	PoolTypeCeph    PoolType = "rbd"     // Ceph RBD
	PoolTypeISCSI   PoolType = "iscsi"   // iSCSI target
	PoolTypeGluster PoolType = "gluster" // GlusterFS
)

// VolumeType represents the purpose of a storage volume.
type VolumeType string

const (
	VolumeTypeBoot      VolumeType = "boot"       // Boot disk volume
	VolumeTypeData      VolumeType = "data"       // Data disk volume
	VolumeTypeCloudInit VolumeType = "cloudinit"  // Cloud-init ISO volume
	VolumeTypeBaseImage VolumeType = "base-image" // Base OS image volume
)

// VolumeFormat represents the disk format.
type VolumeFormat string

const (
	VolumeFormatQCOW2 VolumeFormat = "qcow2" // QCOW2 format
	VolumeFormatRaw   VolumeFormat = "raw"   // Raw format
)

// VolumeSpec specifies how to create a storage volume.
type VolumeSpec struct {
	Name          string       // Volume name (e.g., "my-vm_boot", "fedora-43")
	Type          VolumeType   // Volume type
	Format        VolumeFormat // Disk format (qcow2, raw)
	CapacityGB    uint64       // Capacity in GB
	BackingVolume string       // Optional: backing volume name for qcow2 snapshots
}

// Validate checks if the volume spec is valid.
func (v *VolumeSpec) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("volume name is required")
	}
	if v.Type == "" {
		return fmt.Errorf("volume type is required")
	}
	if v.Format == "" {
		return fmt.Errorf("volume format is required")
	}
	if v.Format != VolumeFormatQCOW2 && v.Format != VolumeFormatRaw {
		return fmt.Errorf("invalid volume format: %s (must be qcow2 or raw)", v.Format)
	}
	if v.CapacityGB == 0 && v.Type != VolumeTypeCloudInit {
		return fmt.Errorf("volume capacity must be greater than 0")
	}
	if v.BackingVolume != "" && v.Format != VolumeFormatQCOW2 {
		return fmt.Errorf("backing volumes are only supported for qcow2 format")
	}
	return nil
}

// PoolInfo contains information about a storage pool.
type PoolInfo struct {
	Name       string   // Pool name
	Type       PoolType // Pool type
	Path       string   // Pool path (for dir-based pools)
	UUID       string   // Pool UUID
	State      string   // Pool state (running, stopped, etc.)
	Autostart  bool     // Whether pool auto-starts on boot
	Persistent bool     // Whether pool is persistent
	Capacity   uint64   // Total capacity in bytes
	Allocation uint64   // Allocated space in bytes
	Available  uint64   // Available space in bytes
}

// CapacityGB returns the pool capacity in GB.
func (p *PoolInfo) CapacityGB() float64 {
	return float64(p.Capacity) / (1024 * 1024 * 1024)
}

// AllocationGB returns the pool allocation in GB.
func (p *PoolInfo) AllocationGB() float64 {
	return float64(p.Allocation) / (1024 * 1024 * 1024)
}

// AvailableGB returns the pool available space in GB.
func (p *PoolInfo) AvailableGB() float64 {
	return float64(p.Available) / (1024 * 1024 * 1024)
}

// VolumeInfo contains information about a storage volume.
type VolumeInfo struct {
	Name       string       // Volume name
	Type       VolumeType   // Volume type
	Format     VolumeFormat // Disk format
	Path       string       // Full path to volume
	Pool       string       // Pool name
	Capacity   uint64       // Capacity in bytes
	Allocation uint64       // Allocated space in bytes
}

// CapacityGB returns the volume capacity in GB.
func (v *VolumeInfo) CapacityGB() float64 {
	return float64(v.Capacity) / (1024 * 1024 * 1024)
}

// AllocationGB returns the volume allocation in GB.
func (v *VolumeInfo) AllocationGB() float64 {
	return float64(v.Allocation) / (1024 * 1024 * 1024)
}

// Default pool configuration.
const (
	// DefaultImagesPool is the pool name for base OS images.
	DefaultImagesPool = "foundry-images"
	// DefaultVMsPool is the pool name for VM disks.
	DefaultVMsPool = "foundry-vms"
	// DefaultImagesPath is the default path for base images.
	DefaultImagesPath = "/var/lib/libvirt/images/foundry/images"
	// DefaultVMsPath is the default path for VM disks.
	DefaultVMsPath = "/var/lib/libvirt/images/foundry/vms"
)
