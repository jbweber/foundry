// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/metadata"
	"github.com/jbweber/foundry/internal/status"
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

// ListVMs lists all VMs and returns them as v1alpha1.VirtualMachine objects
// with their spec loaded from metadata and status populated from libvirt state.
func ListVMs(ctx context.Context) ([]*v1alpha1.VirtualMachine, error) {
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

	return listVMsWithDeps(ctx, libvirtClient.Libvirt())
}

// listVMsWithDeps lists VMs with injected dependencies.
func listVMsWithDeps(_ context.Context, lv libvirtClient) ([]*v1alpha1.VirtualMachine, error) {
	// List all domains (running and stopped)
	domains, _, err := lv.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	// If no domains, return empty slice
	if len(domains) == 0 {
		return []*v1alpha1.VirtualMachine{}, nil
	}

	// Collect VirtualMachine objects for each domain
	vms := make([]*v1alpha1.VirtualMachine, 0, len(domains))
	for _, domain := range domains {
		// Convert interface to *libvirt.Libvirt for metadata operations
		libvirtPtr, ok := lv.(*libvirt.Libvirt)
		if !ok {
			log.Printf("Warning: cannot convert libvirtClient to *libvirt.Libvirt")
			continue
		}

		vm, err := getVirtualMachine(libvirtPtr, domain)
		if err != nil {
			log.Printf("Warning: failed to get VM info for domain %s: %v", domain.Name, err)
			continue
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

// getVirtualMachine loads a VirtualMachine from libvirt metadata and populates
// its status from the current domain state.
func getVirtualMachine(lv *libvirt.Libvirt, domain libvirt.Domain) (*v1alpha1.VirtualMachine, error) {
	// Try to load metadata first
	vm, err := metadata.Load(lv, domain)
	if err != nil {
		// If metadata doesn't exist, create a minimal VM object
		// This handles VMs not created by Foundry
		log.Printf("Warning: no Foundry metadata for %s, creating minimal VM object", domain.Name)
		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: v1alpha1.ObjectMeta{
				Name: domain.Name,
			},
		}
		// Set TypeMeta
		v1alpha1.SetDefaultAPIVersion(vm)
	}

	// Populate status from current domain state
	if err := populateStatus(lv, domain, vm); err != nil {
		return nil, fmt.Errorf("failed to populate status: %w", err)
	}

	return vm, nil
}

// populateStatus updates the VM status based on current libvirt domain state.
func populateStatus(lv *libvirt.Libvirt, domain libvirt.Domain, vm *v1alpha1.VirtualMachine) error {
	// Get domain state
	state, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to get domain state: %w", err)
	}

	// Map libvirt state to VM phase
	phase := mapStateToPhase(state)
	vm.Status.Phase = phase

	// Update Ready condition based on state
	readyStatus := v1alpha1.ConditionFalse
	reason := "NotRunning"
	message := fmt.Sprintf("VM is in state: %s", stateToString(state))

	if state == 1 { // running
		readyStatus = v1alpha1.ConditionTrue
		reason = "Running"
		message = "VM is running"
	}

	status.SetCondition(vm, v1alpha1.ConditionReady, readyStatus, reason, message)

	// TODO: Populate addresses from network interfaces
	// For now, this would require parsing the domain XML or querying the guest agent
	// We'll add this in a future enhancement

	return nil
}

// mapStateToPhase maps libvirt domain state to VirtualMachine phase.
func mapStateToPhase(state int32) v1alpha1.VMPhase {
	switch state {
	case 0: // no state
		return v1alpha1.VMPhasePending
	case 1: // running
		return v1alpha1.VMPhaseRunning
	case 2, 3: // blocked, paused
		return v1alpha1.VMPhaseRunning // Still counts as running
	case 4: // shutdown (in progress)
		return v1alpha1.VMPhaseStopping
	case 5: // shutoff
		return v1alpha1.VMPhaseStopped
	case 6: // crashed
		return v1alpha1.VMPhaseFailed
	case 7: // pmsuspended
		return v1alpha1.VMPhaseRunning
	default:
		return v1alpha1.VMPhasePending // Use Pending for unknown states
	}
}

// GetVM retrieves a single VM by name.
func GetVM(ctx context.Context, name string) (*v1alpha1.VirtualMachine, error) {
	// Connect to libvirt
	libvirtClient, err := foundrylibvirt.ConnectWithContext(ctx, "", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() {
		if err := libvirtClient.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()

	// Look up domain by name
	domain, err := libvirtClient.Libvirt().DomainLookupByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM %s: %w", name, err)
	}

	// Get VM with populated status
	return getVirtualMachine(libvirtClient.Libvirt(), domain)
}
