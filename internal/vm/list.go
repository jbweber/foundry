// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"

	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
)

// VMInfo represents information about a VM.
type VMInfo struct {
	Name      string
	State     string
	Autostart bool
	CPUs      uint16
	MemoryMB  uint64
}

// List lists all VMs (both running and stopped).
//
// Returns a slice of VMInfo structs containing details about each VM.
func List(ctx context.Context) ([]VMInfo, error) {
	// Connect to libvirt
	log.Printf("Connecting to libvirt...")
	libvirtClient, err := foundrylibvirt.ConnectWithContext(ctx, "", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() {
		if err := libvirtClient.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()

	// Delegate to internal function with dependencies
	return listWithDeps(ctx, libvirtClient.Libvirt())
}

// listWithDeps lists VMs with injected dependencies.
// This allows for testing by accepting interfaces instead of concrete types.
func listWithDeps(_ context.Context, lv libvirtClient) ([]VMInfo, error) {
	// List all domains (running and stopped)
	// NeedResults: 1 means populate the domains slice
	// Flags: 0 means all domains (active and inactive)
	domains, _, err := lv.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	// If no domains, return empty slice
	if len(domains) == 0 {
		return []VMInfo{}, nil
	}

	// Collect info for each domain
	vms := make([]VMInfo, 0, len(domains))
	for _, domain := range domains {
		info, err := getDomainInfo(lv, domain)
		if err != nil {
			log.Printf("Warning: failed to get info for domain %s: %v", domain.Name, err)
			continue
		}
		vms = append(vms, info)
	}

	return vms, nil
}

// getDomainInfo gets detailed information about a single domain.
func getDomainInfo(lv libvirtClient, domain libvirt.Domain) (VMInfo, error) {
	// Get domain state
	state, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return VMInfo{}, fmt.Errorf("failed to get domain state: %w", err)
	}

	// Get domain info (CPU, memory)
	stateVal, maxMem, memory, nrVirtCPU, cpuTime, err := lv.DomainGetInfo(domain)
	if err != nil {
		return VMInfo{}, fmt.Errorf("failed to get domain info: %w", err)
	}

	// Get autostart status
	autostart, err := lv.DomainGetAutostart(domain)
	if err != nil {
		log.Printf("Warning: failed to get autostart for %s: %v", domain.Name, err)
		autostart = 0
	}

	// Sanity check: state from GetState and GetInfo should match
	if int32(stateVal) != state {
		log.Printf("Warning: state mismatch for %s: GetState=%d, GetInfo=%d", domain.Name, state, stateVal)
	}

	// Convert memory from KiB to MiB
	memoryMB := memory / 1024

	// Unused variables (keep for future enhancements)
	_ = maxMem
	_ = cpuTime

	return VMInfo{
		Name:      domain.Name,
		State:     stateToString(state),
		Autostart: autostart != 0,
		CPUs:      nrVirtCPU,
		MemoryMB:  memoryMB,
	}, nil
}

// stateToString converts libvirt domain state to human-readable string.
func stateToString(state int32) string {
	switch state {
	case 0:
		return "no state"
	case 1:
		return "running"
	case 2:
		return "blocked"
	case 3:
		return "paused"
	case 4:
		return "shutdown"
	case 5:
		return "shutoff"
	case 6:
		return "crashed"
	case 7:
		return "pmsuspended"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// PrintVMs prints a formatted table of VMs to stdout.
func PrintVMs(vms []VMInfo) {
	if len(vms) == 0 {
		fmt.Println("No VMs found")
		return
	}

	// Create tabwriter for aligned columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATE\tAUTOSTART\tCPUs\tMEMORY")

	for _, vm := range vms {
		autostart := "no"
		if vm.Autostart {
			autostart = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d MiB\n",
			vm.Name, vm.State, autostart, vm.CPUs, vm.MemoryMB)
	}

	_ = w.Flush()
}
