// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/metadata"
	"github.com/jbweber/foundry/internal/naming"
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
//  1. Check if VM exists and is foundry's (its spec is in the domain's metadata)
//  2. Get VM state
//  3. Graceful shutdown if running (5s timeout)
//  4. Force destroy if still running
//  5. Undefine domain (with NVRAM cleanup for UEFI VMs)
//  6. Delete the VM's storage volumes (see vmVolumes)
//
// Only foundry's VMs are destroyed: a domain without foundry's spec in its
// metadata, or whose spec cannot be read, is refused with nothing changed,
// unless force, which destroys it as any other.
//
// Volume cleanup is best-effort - if volumes can't be deleted, warnings are logged
// but the operation continues.
//
// Returns an error if the VM doesn't exist, is refused, or if critical libvirt
// operations fail.
func Destroy(ctx context.Context, vmName string, force bool) error {
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
	return destroyWithDeps(ctx, vmName, LibvirtClient.Libvirt(), storageMgr, force)
}

// destroyWithDeps destroys a VM with injected dependencies.
// This allows for testing by accepting interfaces instead of concrete types.
func destroyWithDeps(ctx context.Context, vmName string, lv LibvirtClient, sm storageManager, force bool) error {
	// Step 1: Check if VM exists
	log.Printf("Looking up VM '%s'...", vmName)
	domain, err := lv.DomainLookupByName(vmName)
	if err != nil {
		return fmt.Errorf("VM '%s' not found: %w", vmName, err)
	}

	// Only foundry's VMs (a domain with foundry's spec in its metadata) are
	// destroyed, unless force: a domain made by other means is refused, and
	// so is one whose spec cannot be read, with nothing changed. The spec
	// also says which pool holds the VM's disks; a forced destroy of a
	// domain without one searches the VMs pool only.
	spec, err := metadata.NewClient(lv).Load(domain)
	if err != nil {
		switch {
		case !force && metadata.IsNotFound(err):
			return fmt.Errorf("VM '%s' is not foundry's (its domain has no foundry spec); nothing changed, use --force to destroy it anyway", vmName)
		case !force:
			return fmt.Errorf("VM '%s': cannot read its foundry spec, nothing changed (use --force to destroy it anyway): %w", vmName, err)
		case !metadata.IsNotFound(err):
			log.Printf("Warning: %v; looking for the VM's volumes in %s only", err, storage.DefaultVMsPool)
		default:
			log.Printf("Warning: VM '%s' is not foundry's; destroying it as forced", vmName)
		}
		spec = nil
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
	log.Printf("Cleaning up storage volumes...")
	deletedCount := deleteVolumes(ctx, sm, vmVolumes(ctx, lv, sm, vmName, spec))

	log.Printf("VM '%s' destroyed successfully (%d volumes deleted)", vmName, deletedCount)
	return nil
}

// poolVolume is one volume, by pool and name.
type poolVolume struct {
	pool, name string
}

// vmVolumes returns a VM's volumes. In the VMs pool, those are the volumes
// named "<vmName>_..." (boot, data, cloud-init and input disks), so that what
// a failed create left behind goes too. In any other pool the VM's spec puts
// its disks in (spec.storagePool, which may be the images pool), only the
// names foundry gives this VM's own volumes are taken (ownVolume), never
// anything else that happens to start with "<vmName>_": image names are free
// text, so an image "rhel_10.qcow2" would otherwise go with a VM named "rhel",
// and the images pool also holds Secure Boot key profiles, which belong to no
// VM. With no spec (a forced destroy of a domain foundry did not make), only
// the VMs pool is searched.
//
// Left out everywhere are the volumes of another defined domain whose own
// name starts with "<vmName>_" (destroying "web" does not take
// "web_2_boot.qcow2" from a VM "web_2"). Pools that cannot be listed are
// skipped with a warning.
func vmVolumes(ctx context.Context, lv LibvirtClient, sm storageManager, vmName string, spec *v1alpha1.VirtualMachine) []poolVolume {
	prefix := vmName + "_"
	var others []string
	if domains, _, err := lv.ConnectListAllDomains(1, 0); err != nil {
		log.Printf("Warning: failed to list domains: %v", err)
	} else {
		for _, d := range domains {
			if strings.HasPrefix(d.Name, prefix) {
				others = append(others, d.Name+"_")
			}
		}
	}

	type search struct {
		pool  string
		match func(volume string) bool
	}
	searches := []search{{storage.DefaultVMsPool, func(volume string) bool {
		_, _, keyProfile := naming.ParseVolumeNameVarstore(volume)
		return strings.HasPrefix(volume, prefix) && !keyProfile
	}}}
	if spec != nil {
		if pool := getStoragePool(spec); pool != storage.DefaultVMsPool {
			searches = append(searches, search{pool, func(volume string) bool { return ownVolume(spec, vmName, volume) }})
		}
	}

	var found []poolVolume
	for _, s := range searches {
		volumes, err := sm.ListVolumes(ctx, s.pool)
		if err != nil {
			log.Printf("Warning: failed to list volumes in pool %s: %v", s.pool, err)
			continue
		}
	volume:
		for _, vol := range volumes {
			if !s.match(vol.Name) {
				continue
			}
			for _, other := range others {
				if strings.HasPrefix(vol.Name, other) {
					continue volume
				}
			}
			found = append(found, poolVolume{pool: s.pool, name: vol.Name})
		}
	}
	return found
}

// ownVolume reports whether volume is one foundry makes for the VM vmName
// with this spec: its boot disk and cloud-init ISO, or a data or input disk
// the spec names.
func ownVolume(spec *v1alpha1.VirtualMachine, vmName, volume string) bool {
	switch volume {
	case naming.VolumeNameBoot(vmName), naming.VolumeNameCloudInit(vmName):
		return true
	}
	for _, d := range spec.Spec.DataDisks {
		if volume == naming.VolumeNameData(vmName, d.Device) {
			return true
		}
	}
	for _, in := range spec.Spec.InputDisks {
		if volume == naming.VolumeNameInput(vmName, in.Label) {
			return true
		}
	}
	return false
}

// deleteVolumes deletes volumes, best effort; returns how many it deleted.
func deleteVolumes(ctx context.Context, sm storageManager, volumes []poolVolume) int {
	deleted := 0
	for _, v := range volumes {
		log.Printf("Deleting volume %s from pool %s...", v.name, v.pool)
		if err := sm.DeleteVolume(ctx, v.pool, v.name); err != nil {
			log.Printf("Warning: failed to delete volume %s: %v", v.name, err)
			continue
		}
		deleted++
	}
	return deleted
}

// TODO(future): Add "repave" operation that replaces only boot disk and cloud-init ISO
// while preserving data disks. This would be useful for OS upgrades without data loss.
// Workflow: stop VM → delete boot volume → delete cloudinit volume → recreate both →
// redefine domain → start VM. Data volumes remain untouched.
