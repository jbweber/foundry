package disk

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/jbweber/foundry/internal/config"
)

// NOTE: This implementation uses qemu-img commands and direct filesystem operations
// rather than libvirt storage pools. This matches the homestead Ansible implementation
// and is simpler to implement and maintain.
//
// Future enhancement: Investigate if libvirt can manage disk creation directly without
// storage pools (e.g., how virt-install creates images). This might provide better
// integration with libvirt's volume management and permissions handling.

const (
	// DefaultStorageBase is the default base directory for VM storage
	DefaultStorageBase = "/var/lib/libvirt/images"

	// QemuUser is the user that owns VM disk files
	QemuUser = "qemu"

	// QemuGroup is the group that owns VM disk files
	QemuGroup = "qemu"

	// DirPermissions are the permissions for VM directories
	DirPermissions = 0755

	// FilePermissions are the permissions for VM disk files
	FilePermissions = 0644
)

// Manager handles storage operations for VMs
type Manager struct {
	storageBase string
	qemuUID     int
	qemuGID     int
}

// NewManager creates a new storage manager
func NewManager() (*Manager, error) {
	return NewManagerWithBase(DefaultStorageBase)
}

// NewManagerWithBase creates a new storage manager with a custom storage base path
func NewManagerWithBase(storageBase string) (*Manager, error) {
	// Look up qemu user and group IDs
	qemuUser, err := user.Lookup(QemuUser)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup %s user: %w", QemuUser, err)
	}

	qemuUID, err := strconv.Atoi(qemuUser.Uid)
	if err != nil {
		return nil, fmt.Errorf("invalid UID for %s user: %w", QemuUser, err)
	}

	qemuGID, err := strconv.Atoi(qemuUser.Gid)
	if err != nil {
		return nil, fmt.Errorf("invalid GID for %s user: %w", QemuUser, err)
	}

	return &Manager{
		storageBase: storageBase,
		qemuUID:     qemuUID,
		qemuGID:     qemuGID,
	}, nil
}

// CreateVMDirectory creates the VM storage directory with proper permissions
func (m *Manager) CreateVMDirectory(vmName string) error {
	vmDir := filepath.Join(m.storageBase, vmName)

	// Create directory
	if err := os.MkdirAll(vmDir, DirPermissions); err != nil {
		return fmt.Errorf("failed to create VM directory %s: %w", vmDir, err)
	}

	// Set ownership
	if err := os.Chown(vmDir, m.qemuUID, m.qemuGID); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", vmDir, err)
	}

	return nil
}

// CreateBootDisk creates a boot disk using qemu-img
func (m *Manager) CreateBootDisk(cfg *config.VMConfig) error {
	if cfg == nil {
		return fmt.Errorf("VM configuration cannot be nil")
	}

	diskPath := filepath.Join(m.storageBase, cfg.GetBootDiskPath())

	var cmd *exec.Cmd

	if cfg.BootDisk.Image != "" {
		// Create boot disk with backing file (snapshot/overlay)
		// This allows the VM to use a base image without modifying it
		cmd = exec.Command(
			"qemu-img", "create",
			"-f", "qcow2",
			"-b", cfg.BootDisk.Image,
			"-F", "qcow2", // Assume backing file is qcow2
			diskPath,
			fmt.Sprintf("%dG", cfg.BootDisk.SizeGB),
		)
	} else {
		// Create empty boot disk
		cmd = exec.Command(
			"qemu-img", "create",
			"-f", "qcow2",
			diskPath,
			fmt.Sprintf("%dG", cfg.BootDisk.SizeGB),
		)
	}

	// Execute qemu-img command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create boot disk %s: %w\nOutput: %s", diskPath, err, string(output))
	}

	// Set ownership and permissions
	if err := m.setFileOwnership(diskPath); err != nil {
		return err
	}

	return nil
}

// CreateDataDisk creates a data disk using qemu-img
func (m *Manager) CreateDataDisk(vmName string, disk config.DataDiskConfig) error {
	diskPath := filepath.Join(m.storageBase, vmName, fmt.Sprintf("data-%s.qcow2", disk.Device))

	// Create empty data disk
	cmd := exec.Command(
		"qemu-img", "create",
		"-f", "qcow2",
		diskPath,
		fmt.Sprintf("%dG", disk.SizeGB),
	)

	// Execute qemu-img command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create data disk %s: %w\nOutput: %s", diskPath, err, string(output))
	}

	// Set ownership and permissions
	if err := m.setFileOwnership(diskPath); err != nil {
		return err
	}

	return nil
}

// WriteCloudInitISO writes the cloud-init ISO to disk
func (m *Manager) WriteCloudInitISO(cfg *config.VMConfig, isoData []byte) error {
	if cfg == nil {
		return fmt.Errorf("VM configuration cannot be nil")
	}

	if len(isoData) == 0 {
		return fmt.Errorf("ISO data cannot be empty")
	}

	isoPath := filepath.Join(m.storageBase, cfg.GetCloudInitISOPath())

	// Write ISO file
	if err := os.WriteFile(isoPath, isoData, FilePermissions); err != nil {
		return fmt.Errorf("failed to write cloud-init ISO %s: %w", isoPath, err)
	}

	// Set ownership
	if err := os.Chown(isoPath, m.qemuUID, m.qemuGID); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", isoPath, err)
	}

	return nil
}

// DeleteVM removes the entire VM directory and all its contents
func (m *Manager) DeleteVM(vmName string) error {
	vmDir := filepath.Join(m.storageBase, vmName)

	// Check if directory exists
	if _, err := os.Stat(vmDir); os.IsNotExist(err) {
		// Directory doesn't exist, nothing to do
		return nil
	}

	// Remove directory and all contents
	if err := os.RemoveAll(vmDir); err != nil {
		return fmt.Errorf("failed to delete VM directory %s: %w", vmDir, err)
	}

	return nil
}

// CheckDiskSpace verifies that sufficient disk space is available
func (m *Manager) CheckDiskSpace(cfg *config.VMConfig) error {
	if cfg == nil {
		return fmt.Errorf("VM configuration cannot be nil")
	}

	// Calculate total disk space needed (in GB)
	totalNeeded := cfg.BootDisk.SizeGB
	for _, disk := range cfg.DataDisks {
		totalNeeded += disk.SizeGB
	}

	// Get filesystem statistics
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.storageBase, &stat); err != nil {
		return fmt.Errorf("failed to get filesystem stats for %s: %w", m.storageBase, err)
	}

	// Calculate available space in GB
	availableGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)

	// Check if we have enough space (with some buffer)
	if uint64(totalNeeded) > availableGB {
		return fmt.Errorf("insufficient disk space: need %dGB, have %dGB available", totalNeeded, availableGB)
	}

	return nil
}

// GetVMDirectory returns the full path to the VM's storage directory
func (m *Manager) GetVMDirectory(vmName string) string {
	return filepath.Join(m.storageBase, vmName)
}

// VMDirectoryExists checks if the VM directory already exists
func (m *Manager) VMDirectoryExists(vmName string) (bool, error) {
	vmDir := m.GetVMDirectory(vmName)

	info, err := os.Stat(vmDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check VM directory %s: %w", vmDir, err)
	}

	return info.IsDir(), nil
}

// setFileOwnership sets qemu:qemu ownership on a file
func (m *Manager) setFileOwnership(path string) error {
	if err := os.Chown(path, m.qemuUID, m.qemuGID); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", path, err)
	}

	// Ensure proper permissions
	if err := os.Chmod(path, FilePermissions); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}

	return nil
}
