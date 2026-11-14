// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"errors"
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

// shutdownTimeout is how long to wait for graceful shutdown before forcing.
// A variable so tests can shorten it.
var shutdownTimeout = 5 * time.Second

const (
	// Domain states (from libvirt VIR_DOMAIN_* constants)
	domainStateRunning = 1
	domainStateShutoff = 5
)

// Destroy destroys a VM by name.
//
// This orchestrates the entire VM destruction process:
//  1. Check if VM exists and is foundry's (its spec is in the domain's metadata),
//     and list the domains whose volumes it must spare (see vmVolumes)
//  2. Get VM state
//  3. Stop it in any state but shut off: graceful shutdown (5s timeout),
//     or at once a forced destroy when paused, suspended or crashed
//  4. Force destroy if still not shut off
//  5. Undefine domain (with NVRAM cleanup for UEFI VMs)
//  6. Delete the VM's storage volumes (see vmVolumes)
//  7. Delete its serial console log from the logs pool, with the copies
//     virtlogd rotated it into (exact names, see naming.SerialLogCopy)
//
// Only foundry's VMs are destroyed: a domain without foundry's spec in its
// metadata, or whose spec cannot be read, is refused with nothing changed,
// unless force, which destroys it as any other.
//
// Volume cleanup is best-effort - if volumes can't be deleted, warnings are logged
// but the operation continues.
//
// A name with no domain but volumes left of one (a create killed before it
// defined its domain) has those volumes removed: see destroyLeftovers.
//
// Returns an error if the VM doesn't exist (no domain and nothing left of
// one), is refused, or if critical libvirt
// operations fail. A VM that cannot be stopped, or whose state cannot be
// read, and domains that cannot be listed, are errors with the definition
// and volumes left as they are, also with force.
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
		var le libvirt.Error
		if errors.As(err, &le) && le.Code == uint32(libvirt.ErrNoDomain) {
			return destroyLeftovers(ctx, vmName, lv, sm, err)
		}
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

	// Step 1b: List the other domains whose names start with "<vm>_", whose
	// volumes destroy must spare, before anything is changed: without the
	// list, destroying "web" would take the "web_..." volumes of a VM
	// "web_2", so a failed listing stops the destroy, also when forced
	others, err := otherVMPrefixes(lv, vmName)
	if err != nil {
		return fmt.Errorf("VM '%s': cannot list the domains whose volumes it must keep, nothing changed: %w", vmName, err)
	}

	// Step 2: Get VM state
	log.Printf("Checking VM state...")
	state, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to get VM state: %w", err)
	}

	// Steps 3-4: stop the domain in any state but shut off (running,
	// blocked, paused, shutting down, crashed, suspended): undefining an
	// active domain does not stop it, it only makes it transient, and its
	// disks would be deleted under it. One that cannot be stopped is an
	// error, with the definition and volumes left as they are.
	// An interrupt (Ctrl-C) before the domain is undefined stops the
	// destroy with nothing undefined or deleted; the graceful shutdown's
	// wait is not cut short by it, so an interrupt never turns into a
	// forced stop. After the undefine the destroy runs to its end.
	if ctx.Err() != nil {
		return fmt.Errorf("VM '%s': interrupted, nothing changed", vmName)
	}
	if state != domainStateShutoff {
		if err := stopActiveDomain(context.WithoutCancel(ctx), lv, domain, state, shutdownTimeout); err != nil {
			return fmt.Errorf("VM '%s' could not be stopped, nothing deleted: %w", vmName, err)
		}
	}
	if ctx.Err() != nil {
		return fmt.Errorf("VM '%s': interrupted after it was stopped; nothing undefined or deleted, run destroy again to finish", vmName)
	}
	ctx = context.WithoutCancel(ctx)

	// Step 5: Undefine domain with NVRAM cleanup
	log.Printf("Undefining domain...")
	if err := lv.DomainUndefineFlags(domain, libvirt.DomainUndefineNvram); err != nil {
		return fmt.Errorf("failed to undefine domain: %w", err)
	}

	// Step 6: Delete storage volumes
	log.Printf("Cleaning up storage volumes...")
	deletedCount := deleteVolumes(ctx, sm, vmVolumes(ctx, sm, vmName, spec, others))

	// Step 7: Delete the VM's serial log and virtlogd's rotated copies of it
	// (exact names; the domain is stopped, so virtlogd has closed them)
	deletedCount += deleteVolumes(ctx, sm, serialLogVolumes(ctx, sm, vmName))

	log.Printf("VM '%s' destroyed successfully (%d volumes deleted)", vmName, deletedCount)
	return nil
}

// destroyLeftovers removes what is left of a VM whose domain is not defined:
// its volumes in the VMs pool, as destroy finds them ("<vmName>_...", less
// those of another defined VM whose name starts with "<vmName>_"), and its
// serial log. With no
// spec there is no other pool to search. A VM is in that state after a
// create killed before it defined its domain (a second Ctrl-C, a crash), or
// when the domain was undefined by other means; create refuses the name
// ("boot volume already exists") until the volumes are gone. With nothing
// left, the name is unknown and notFound is returned, as before.
func destroyLeftovers(ctx context.Context, vmName string, lv LibvirtClient, sm storageManager, notFound error) error {
	log.Printf("VM '%s' is not defined; looking for volumes left of it...", vmName)
	others, err := otherVMPrefixes(lv, vmName)
	if err != nil {
		return fmt.Errorf("VM '%s': cannot list the domains whose volumes it must keep, nothing changed: %w", vmName, err)
	}
	volumes := vmVolumes(ctx, sm, vmName, nil, others)
	volumes = append(volumes, serialLogVolumes(ctx, sm, vmName)...)
	if len(volumes) == 0 {
		return fmt.Errorf("VM '%s' not found: %w", vmName, notFound)
	}
	deleted := deleteVolumes(context.WithoutCancel(ctx), sm, volumes)
	log.Printf("Leftovers of VM '%s' removed (%d of %d volumes deleted)", vmName, deleted, len(volumes))
	if deleted < len(volumes) {
		return fmt.Errorf("VM '%s': %d of %d leftover volumes could not be deleted", vmName, len(volumes)-deleted, len(volumes))
	}
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
// Left out everywhere are the volumes starting with one of others, the
// prefixes of the other defined domains whose names start with "<vmName>_"
// (destroying "web" does not take "web_2_boot.qcow2" from a VM "web_2"; see
// otherVMPrefixes). Pools that cannot be listed are
// skipped with a warning.
func vmVolumes(ctx context.Context, sm storageManager, vmName string, spec *v1alpha1.VirtualMachine, others []string) []poolVolume {
	prefix := vmName + "_"

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

// otherVMPrefixes returns the volume prefixes ("<name>_") of the other
// defined domains whose names start with "<vmName>_": their volumes also
// start with "<vmName>_", and vmVolumes leaves them out. An error means the
// domains could not be listed, and so which volumes are another VM's is
// unknown.
func otherVMPrefixes(lv LibvirtClient, vmName string) ([]string, error) {
	domains, _, err := lv.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}
	prefix := vmName + "_"
	var others []string
	for _, d := range domains {
		if strings.HasPrefix(d.Name, prefix) {
			others = append(others, d.Name+"_")
		}
	}
	return others, nil
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

// stopActiveDomain stops a domain that is not shut off. A paused,
// suspended (pmsuspended) or crashed guest cannot answer the ACPI power
// button, so it is destroyed at once; in any other state it gets the
// graceful shutdown of stopDomain first.
func stopActiveDomain(ctx context.Context, lv LibvirtClient, domain libvirt.Domain, state int32, timeout time.Duration) error {
	switch libvirt.DomainState(state) {
	case libvirt.DomainPaused, libvirt.DomainPmsuspended, libvirt.DomainCrashed:
		log.Printf("VM is %s; destroying it (it cannot shut down by itself)...", domainStateName(state))
		if err := lv.DomainDestroy(domain); err != nil {
			return fmt.Errorf("force destroy failed: %w", err)
		}
		return nil
	}
	return stopDomain(ctx, lv, domain, timeout)
}

// domainStateName names the libvirt domain states destroy reports.
func domainStateName(state int32) string {
	switch libvirt.DomainState(state) {
	case libvirt.DomainPaused:
		return "paused"
	case libvirt.DomainPmsuspended:
		return "suspended"
	case libvirt.DomainCrashed:
		return "crashed"
	}
	return fmt.Sprintf("in state %d", state)
}

// stopDomain stops a running domain: a graceful shutdown (ACPI power button),
// up to timeout, then a forced destroy if it has not shut off by then. It
// returns an error when the domain may still be running: the forced destroy
// failed, or its state could not be read.
func stopDomain(ctx context.Context, lv LibvirtClient, domain libvirt.Domain, timeout time.Duration) error {
	log.Printf("VM is running, attempting graceful shutdown...")
	if err := lv.DomainShutdown(domain); err != nil {
		log.Printf("Warning: graceful shutdown failed: %v", err)
	} else {
		log.Printf("Waiting up to %v for graceful shutdown...", timeout)
		shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

	wait:
		for {
			select {
			case <-shutdownCtx.Done():
				log.Printf("Graceful shutdown timed out")
				break wait
			case <-ticker.C:
				currentState, _, err := lv.DomainGetState(domain, 0)
				if err != nil {
					log.Printf("Warning: failed to check shutdown state: %v", err)
					break wait
				}
				if currentState == domainStateShutoff {
					log.Printf("VM shut down gracefully")
					return nil
				}
			}
		}
	}

	// Force destroy if still not shut off
	currentState, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to check state before destroy: %w", err)
	}
	if currentState == domainStateShutoff {
		return nil
	}
	log.Printf("Force destroying VM...")
	if err := lv.DomainDestroy(domain); err != nil {
		return fmt.Errorf("force destroy failed: %w", err)
	}
	return nil
}
