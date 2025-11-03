package vm

import (
	"context"
	"fmt"
	"testing"

	"github.com/digitalocean/go-libvirt"
)

func TestListWithDeps_NoDomains(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Default mock returns empty list
	vms, err := listWithDeps(ctx, mock)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vms) != 0 {
		t.Errorf("expected 0 VMs, got %d", len(vms))
	}

	if mock.connectListAllDomainsCalls != 1 {
		t.Errorf("expected 1 ConnectListAllDomains call, got %d", mock.connectListAllDomainsCalls)
	}
}

func TestListWithDeps_SingleVM(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Configure mock to return one VM
	mock.connectListAllDomainsFunc = func(needResults int32, flags libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return []libvirt.Domain{
			{Name: "test-vm"},
		}, 1, nil
	}

	// Configure domain info (running, 2048 MiB, 2 CPUs)
	mock.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return 1, 0, nil // running
	}
	mock.domainGetInfoFunc = func(dom libvirt.Domain) (uint8, uint64, uint64, uint16, uint64, error) {
		// state=running, maxMem=2GiB, mem=2048MiB (in KiB = 2097152), CPUs=2
		return 1, 2097152, 2097152, 2, 123456, nil
	}
	mock.domainGetAutostartFunc = func(dom libvirt.Domain) (int32, error) {
		return 1, nil // autostart enabled
	}

	vms, err := listWithDeps(ctx, mock)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vms) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(vms))
	}

	vm := vms[0]
	if vm.Name != "test-vm" {
		t.Errorf("expected name 'test-vm', got '%s'", vm.Name)
	}
	if vm.State != "running" {
		t.Errorf("expected state 'running', got '%s'", vm.State)
	}
	if !vm.Autostart {
		t.Errorf("expected autostart=true, got false")
	}
	if vm.CPUs != 2 {
		t.Errorf("expected 2 CPUs, got %d", vm.CPUs)
	}
	if vm.MemoryMB != 2048 {
		t.Errorf("expected 2048 MiB memory, got %d", vm.MemoryMB)
	}
}

func TestListWithDeps_MultipleVMs(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Configure mock to return multiple VMs
	mock.connectListAllDomainsFunc = func(needResults int32, flags libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return []libvirt.Domain{
			{Name: "vm1"},
			{Name: "vm2"},
			{Name: "vm3"},
		}, 3, nil
	}

	// Different states and configurations
	mock.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		switch dom.Name {
		case "vm1":
			return 1, 0, nil // running
		case "vm2":
			return 5, 0, nil // shutoff
		case "vm3":
			return 3, 0, nil // paused
		default:
			return 0, 0, nil
		}
	}

	mock.domainGetInfoFunc = func(dom libvirt.Domain) (uint8, uint64, uint64, uint16, uint64, error) {
		switch dom.Name {
		case "vm1":
			return 1, 4194304, 4194304, 4, 0, nil // 4GiB, 4 CPUs
		case "vm2":
			return 5, 2097152, 2097152, 2, 0, nil // 2GiB, 2 CPUs
		case "vm3":
			return 3, 1048576, 1048576, 1, 0, nil // 1GiB, 1 CPU
		default:
			return 0, 0, 0, 0, 0, nil
		}
	}

	mock.domainGetAutostartFunc = func(dom libvirt.Domain) (int32, error) {
		if dom.Name == "vm1" {
			return 1, nil // autostart enabled
		}
		return 0, nil // autostart disabled
	}

	vms, err := listWithDeps(ctx, mock)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vms) != 3 {
		t.Fatalf("expected 3 VMs, got %d", len(vms))
	}

	// Check vm1
	if vms[0].Name != "vm1" {
		t.Errorf("expected vm1, got %s", vms[0].Name)
	}
	if vms[0].State != "running" {
		t.Errorf("vm1: expected state 'running', got '%s'", vms[0].State)
	}
	if !vms[0].Autostart {
		t.Errorf("vm1: expected autostart=true")
	}
	if vms[0].CPUs != 4 {
		t.Errorf("vm1: expected 4 CPUs, got %d", vms[0].CPUs)
	}
	if vms[0].MemoryMB != 4096 {
		t.Errorf("vm1: expected 4096 MiB, got %d", vms[0].MemoryMB)
	}

	// Check vm2
	if vms[1].Name != "vm2" {
		t.Errorf("expected vm2, got %s", vms[1].Name)
	}
	if vms[1].State != "shutoff" {
		t.Errorf("vm2: expected state 'shutoff', got '%s'", vms[1].State)
	}
	if vms[1].Autostart {
		t.Errorf("vm2: expected autostart=false")
	}

	// Check vm3
	if vms[2].Name != "vm3" {
		t.Errorf("expected vm3, got %s", vms[2].Name)
	}
	if vms[2].State != "paused" {
		t.Errorf("vm3: expected state 'paused', got '%s'", vms[2].State)
	}
}

func TestListWithDeps_ListError(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Configure mock to return error
	expectedErr := fmt.Errorf("connection failed")
	mock.connectListAllDomainsFunc = func(needResults int32, flags libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return nil, 0, expectedErr
	}

	vms, err := listWithDeps(ctx, mock)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if vms != nil {
		t.Errorf("expected nil VMs on error, got %v", vms)
	}

	if mock.connectListAllDomainsCalls != 1 {
		t.Errorf("expected 1 ConnectListAllDomains call, got %d", mock.connectListAllDomainsCalls)
	}
}

func TestListWithDeps_GetInfoError(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Configure mock to return multiple VMs
	mock.connectListAllDomainsFunc = func(needResults int32, flags libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return []libvirt.Domain{
			{Name: "vm1"},
			{Name: "vm2-broken"},
			{Name: "vm3"},
		}, 3, nil
	}

	// vm2 has a GetState error - should be skipped
	mock.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		if dom.Name == "vm2-broken" {
			return 0, 0, fmt.Errorf("failed to get state")
		}
		return 1, 0, nil
	}

	vms, err := listWithDeps(ctx, mock)

	if err != nil {
		t.Fatalf("expected no error (partial success), got: %v", err)
	}

	// Should only have 2 VMs (vm2 skipped due to error)
	if len(vms) != 2 {
		t.Fatalf("expected 2 VMs (1 skipped), got %d", len(vms))
	}

	// Check we got vm1 and vm3, not vm2
	names := map[string]bool{}
	for _, vm := range vms {
		names[vm.Name] = true
	}

	if !names["vm1"] {
		t.Errorf("expected vm1 in results")
	}
	if names["vm2-broken"] {
		t.Errorf("vm2-broken should have been skipped due to error")
	}
	if !names["vm3"] {
		t.Errorf("expected vm3 in results")
	}
}

func TestListWithDeps_AutostartError(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()

	// Configure mock to return one VM
	mock.connectListAllDomainsFunc = func(needResults int32, flags libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return []libvirt.Domain{{Name: "test-vm"}}, 1, nil
	}

	// Autostart call fails - should default to false but still succeed
	mock.domainGetAutostartFunc = func(dom libvirt.Domain) (int32, error) {
		return 0, fmt.Errorf("autostart query failed")
	}

	vms, err := listWithDeps(ctx, mock)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(vms) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(vms))
	}

	// Autostart should be false when query fails
	if vms[0].Autostart {
		t.Errorf("expected autostart=false when query fails")
	}
}

func TestStateToString(t *testing.T) {
	tests := []struct {
		state    int32
		expected string
	}{
		{0, "no state"},
		{1, "running"},
		{2, "blocked"},
		{3, "paused"},
		{4, "shutdown"},
		{5, "shutoff"},
		{6, "crashed"},
		{7, "pmsuspended"},
		{99, "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("state_%d", tt.state), func(t *testing.T) {
			result := stateToString(tt.state)
			if result != tt.expected {
				t.Errorf("stateToString(%d) = %s, want %s", tt.state, result, tt.expected)
			}
		})
	}
}

// TestMapStateToPhase tests VM phase mapping
func TestMapStateToPhase(t *testing.T) {
	tests := []struct {
		state         int32
		expectedPhase string
	}{
		{0, "Pending"},  // no state
		{1, "Running"},  // running
		{2, "Running"},  // blocked
		{3, "Running"},  // paused
		{4, "Stopping"}, // shutdown
		{5, "Stopped"},  // shutoff
		{6, "Failed"},   // crashed
		{7, "Running"},  // pmsuspended
		{99, "Pending"}, // unknown
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("state_%d", tt.state), func(t *testing.T) {
			result := mapStateToPhase(tt.state)
			if string(result) != tt.expectedPhase {
				t.Errorf("mapStateToPhase(%d) = %s, want %s", tt.state, result, tt.expectedPhase)
			}
		})
	}
}

// TestPrintVMs tests VM list printing
func TestPrintVMs(t *testing.T) {
	tests := []struct {
		name string
		vms  []VMInfo
	}{
		{
			name: "empty list",
			vms:  []VMInfo{},
		},
		{
			name: "single VM",
			vms: []VMInfo{
				{Name: "test-vm", State: "running", Autostart: true, CPUs: 2, MemoryMB: 2048},
			},
		},
		{
			name: "multiple VMs",
			vms: []VMInfo{
				{Name: "vm1", State: "running", Autostart: true, CPUs: 4, MemoryMB: 4096},
				{Name: "vm2", State: "shutoff", Autostart: false, CPUs: 2, MemoryMB: 2048},
				{Name: "vm3", State: "paused", Autostart: true, CPUs: 1, MemoryMB: 1024},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			PrintVMs(tt.vms)
		})
	}
}
