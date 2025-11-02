// Package vm provides high-level VM lifecycle management operations.
//
// This package orchestrates the various low-level components (config, storage,
// cloud-init, libvirt) to provide simple, high-level operations for managing
// virtual machines.
//
// The main operations are:
//   - Create: Create a new VM from a configuration file
//   - Destroy: Shut down and remove a VM (not yet implemented)
//   - List: List all VMs and their status (not yet implemented)
//
// Error Handling:
//
// All operations use best-effort cleanup on failure. If any step fails during
// VM creation, the package attempts to clean up all partially-created resources
// (storage, libvirt domains, etc.). Cleanup errors are logged but do not cause
// the operation to fail.
//
// Context Support:
//
// All operations accept a context.Context for cancellation support. If the
// context is cancelled during an operation, cleanup is still attempted.
package vm
