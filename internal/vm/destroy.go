// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"

	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/storage"
)

const (
	// shutdownTimeout is how long to wait for graceful shutdown before forcing.
	shutdownTimeout = 5 * time.Second

	// Domain states (from libvirt VIR_DOMAIN_* constants)
	domainStateRunning = 1
	domainStateShutoff = 5
)

// Destroy destroys a VM by name.
//
// This orchestrates the entire VM destruction process:
//  1. Check if VM exists
//  2. Get VM state
//  3. Graceful shutdown if running (5s timeout)
//  4. Force destroy if still running
//  5. Undefine domain (with NVRAM cleanup for UEFI VMs)
//  6. Delete all storage volumes from pool
//
// Volume cleanup is best-effort - if volumes can't be deleted, warnings are logged
// but the operation continues.
//
// Returns an error if the VM doesn't exist or if critical libvirt operations fail.
func Destroy(ctx context.Context, vmName string) error {
	// Connect to libvirt
	log.Printf("Connecting to libvirt...")
	LibvirtClient, err := foundrylibvirt.ConnectWithContext(ctx, "", 0)
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() {
		if err := LibvirtClient.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()

	// Create storage manager
	log.Printf("Initializing storage manager...")
	storageMgr := storage.NewManager(LibvirtClient.Libvirt())

	// Ensure default pools exist (needed for volume listing)
	log.Printf("Ensuring default storage pools exist...")
	if err := storageMgr.EnsureDefaultPools(ctx); err != nil {
		return fmt.Errorf("failed to ensure default pools: %w", err)
	}

	// Delegate to internal function with dependencies
	return destroyWithDeps(ctx, vmName, LibvirtClient.Libvirt(), storageMgr)
}

// destroyWithDeps destroys a VM with injected dependencies.
// This allows for testing by accepting interfaces instead of concrete types.
func destroyWithDeps(ctx context.Context, vmName string, lv LibvirtClient, sm storageManager) error {
	// Step 1: Check if VM exists
	log.Printf("Looking up VM '%s'...", vmName)
	domain, err := lv.DomainLookupByName(vmName)
	if err != nil {
		return fmt.Errorf("VM '%s' not found: %w", vmName, err)
	}

	// Step 2: Get VM state
	log.Printf("Checking VM state...")
	state, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to get VM state: %w", err)
	}

	// Step 3: Graceful shutdown if running
	needsForceDestroy := false
	if state == domainStateRunning {
		log.Printf("VM is running, attempting graceful shutdown...")
		if err := lv.DomainShutdown(domain); err != nil {
			log.Printf("Warning: graceful shutdown failed: %v", err)
			needsForceDestroy = true
		} else {
			// Wait for shutdown with timeout
			log.Printf("Waiting up to %v for graceful shutdown...", shutdownTimeout)
			shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()

			// Poll for shutdown
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			shutdownSucceeded := false
			for !shutdownSucceeded {
				select {
				case <-shutdownCtx.Done():
					// Timeout - will force destroy below
					log.Printf("Graceful shutdown timed out")
					needsForceDestroy = true
					shutdownSucceeded = true // exit loop
				case <-ticker.C:
					currentState, _, err := lv.DomainGetState(domain, 0)
					if err != nil {
						log.Printf("Warning: failed to check shutdown state: %v", err)
						needsForceDestroy = true
						shutdownSucceeded = true // exit loop
					} else if currentState == domainStateShutoff {
						log.Printf("VM shut down gracefully")
						shutdownSucceeded = true // exit loop
					}
				}
			}
		}
	}

	// Step 4: Force destroy if still running
	if needsForceDestroy {
		// Check state one more time
		currentState, _, err := lv.DomainGetState(domain, 0)
		if err != nil {
			log.Printf("Warning: failed to check state before destroy: %v", err)
		}
		if err == nil && currentState == domainStateRunning {
			log.Printf("Force destroying VM...")
			if err := lv.DomainDestroy(domain); err != nil {
				log.Printf("Warning: force destroy failed: %v", err)
			}
		}
	}

	// Step 5: Undefine domain with NVRAM cleanup
	log.Printf("Undefining domain...")
	if err := lv.DomainUndefineFlags(domain, libvirt.DomainUndefineNvram); err != nil {
		return fmt.Errorf("failed to undefine domain: %w", err)
	}

	// Step 6: Delete storage volumes
	// We search for all volumes with the VM name prefix in both default pools
	log.Printf("Cleaning up storage volumes...")
	pools := []string{"foundry-vms", "foundry-images"}
	deletedCount := 0

	for _, poolName := range pools {
		volumes, err := sm.ListVolumes(ctx, poolName)
		if err != nil {
			log.Printf("Warning: failed to list volumes in pool %s: %v", poolName, err)
			continue
		}

		// Find volumes that belong to this VM (prefix match: "{vmName}_")
		vmPrefix := vmName + "_"
		for _, vol := range volumes {
			if strings.HasPrefix(vol.Name, vmPrefix) {
				log.Printf("Deleting volume %s from pool %s...", vol.Name, poolName)
				if err := sm.DeleteVolume(ctx, poolName, vol.Name); err != nil {
					log.Printf("Warning: failed to delete volume %s: %v", vol.Name, err)
				} else {
					deletedCount++
				}
			}
		}
	}

	log.Printf("VM '%s' destroyed successfully (%d volumes deleted)", vmName, deletedCount)
	return nil
}

// TODO(future): Add "repave" operation that replaces only boot disk and cloud-init ISO
// while preserving data disks. This would be useful for OS upgrades without data loss.
// Workflow: stop VM → delete boot volume → delete cloudinit volume → recreate both →
// redefine domain → start VM. Data volumes remain untouched.
