package status

import (
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestTransitionToCreating(t *testing.T) {
	tests := []struct {
		name      string
		phase     v1alpha1.VMPhase
		wantError bool
	}{
		{
			name:      "valid transition from Pending",
			phase:     v1alpha1.VMPhasePending,
			wantError: false,
		},
		{
			name:      "invalid transition from Running",
			phase:     v1alpha1.VMPhaseRunning,
			wantError: true,
		},
		{
			name:      "invalid transition from Stopped",
			phase:     v1alpha1.VMPhaseStopped,
			wantError: true,
		},
		{
			name:      "invalid transition from Failed",
			phase:     v1alpha1.VMPhaseFailed,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := v1alpha1.NewVirtualMachine("test-vm")
			vm.SetPhase(tt.phase)

			err := TransitionToCreating(vm)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				// Phase should not change on error
				if vm.GetPhase() != tt.phase {
					t.Errorf("Phase should not change on error, got %s", vm.GetPhase())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if vm.GetPhase() != v1alpha1.VMPhaseCreating {
					t.Errorf("Expected phase Creating, got %s", vm.GetPhase())
				}
				// Check Ready condition is set to False
				if !IsConditionFalse(vm, v1alpha1.ConditionReady) {
					t.Error("Expected Ready condition to be False during creation")
				}
				cond := GetCondition(vm, v1alpha1.ConditionReady)
				if cond.Reason != "Creating" {
					t.Errorf("Expected reason 'Creating', got %s", cond.Reason)
				}
			}
		})
	}
}

func TestTransitionToRunning(t *testing.T) {
	tests := []struct {
		name      string
		phase     v1alpha1.VMPhase
		wantError bool
	}{
		{
			name:      "valid transition from Creating",
			phase:     v1alpha1.VMPhaseCreating,
			wantError: false,
		},
		{
			name:      "valid transition from Stopped",
			phase:     v1alpha1.VMPhaseStopped,
			wantError: false,
		},
		{
			name:      "invalid transition from Pending",
			phase:     v1alpha1.VMPhasePending,
			wantError: true,
		},
		{
			name:      "invalid transition from Failed",
			phase:     v1alpha1.VMPhaseFailed,
			wantError: true,
		},
		{
			name:      "invalid transition from Stopping",
			phase:     v1alpha1.VMPhaseStopping,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := v1alpha1.NewVirtualMachine("test-vm")
			vm.SetPhase(tt.phase)
			vm.Generation = 5

			err := TransitionToRunning(vm)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				if vm.GetPhase() != tt.phase {
					t.Errorf("Phase should not change on error, got %s", vm.GetPhase())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if vm.GetPhase() != v1alpha1.VMPhaseRunning {
					t.Errorf("Expected phase Running, got %s", vm.GetPhase())
				}
				// Check Ready condition is set to True
				if !IsConditionTrue(vm, v1alpha1.ConditionReady) {
					t.Error("Expected Ready condition to be True")
				}
				// Check ObservedGeneration is updated
				if vm.Status.ObservedGeneration != 5 {
					t.Errorf("Expected ObservedGeneration 5, got %d", vm.Status.ObservedGeneration)
				}
			}
		})
	}
}

func TestTransitionToStopping(t *testing.T) {
	tests := []struct {
		name      string
		phase     v1alpha1.VMPhase
		wantError bool
	}{
		{
			name:      "valid transition from Running",
			phase:     v1alpha1.VMPhaseRunning,
			wantError: false,
		},
		{
			name:      "invalid transition from Pending",
			phase:     v1alpha1.VMPhasePending,
			wantError: true,
		},
		{
			name:      "invalid transition from Creating",
			phase:     v1alpha1.VMPhaseCreating,
			wantError: true,
		},
		{
			name:      "invalid transition from Stopped",
			phase:     v1alpha1.VMPhaseStopped,
			wantError: true,
		},
		{
			name:      "invalid transition from Failed",
			phase:     v1alpha1.VMPhaseFailed,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := v1alpha1.NewVirtualMachine("test-vm")
			vm.SetPhase(tt.phase)

			err := TransitionToStopping(vm)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				if vm.GetPhase() != tt.phase {
					t.Errorf("Phase should not change on error, got %s", vm.GetPhase())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if vm.GetPhase() != v1alpha1.VMPhaseStopping {
					t.Errorf("Expected phase Stopping, got %s", vm.GetPhase())
				}
				// Check Ready condition is set to False
				if !IsConditionFalse(vm, v1alpha1.ConditionReady) {
					t.Error("Expected Ready condition to be False during shutdown")
				}
				cond := GetCondition(vm, v1alpha1.ConditionReady)
				if cond.Reason != "Stopping" {
					t.Errorf("Expected reason 'Stopping', got %s", cond.Reason)
				}
			}
		})
	}
}

func TestTransitionToStopped(t *testing.T) {
	tests := []struct {
		name      string
		phase     v1alpha1.VMPhase
		wantError bool
	}{
		{
			name:      "valid transition from Stopping",
			phase:     v1alpha1.VMPhaseStopping,
			wantError: false,
		},
		{
			name:      "valid transition from Running (forced)",
			phase:     v1alpha1.VMPhaseRunning,
			wantError: false,
		},
		{
			name:      "invalid transition from Pending",
			phase:     v1alpha1.VMPhasePending,
			wantError: true,
		},
		{
			name:      "invalid transition from Creating",
			phase:     v1alpha1.VMPhaseCreating,
			wantError: true,
		},
		{
			name:      "invalid transition from Failed",
			phase:     v1alpha1.VMPhaseFailed,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := v1alpha1.NewVirtualMachine("test-vm")
			vm.SetPhase(tt.phase)

			err := TransitionToStopped(vm)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				if vm.GetPhase() != tt.phase {
					t.Errorf("Phase should not change on error, got %s", vm.GetPhase())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if vm.GetPhase() != v1alpha1.VMPhaseStopped {
					t.Errorf("Expected phase Stopped, got %s", vm.GetPhase())
				}
				// Check Ready condition is set to False
				if !IsConditionFalse(vm, v1alpha1.ConditionReady) {
					t.Error("Expected Ready condition to be False when stopped")
				}
				cond := GetCondition(vm, v1alpha1.ConditionReady)
				if cond.Reason != "Stopped" {
					t.Errorf("Expected reason 'Stopped', got %s", cond.Reason)
				}
			}
		})
	}
}

func TestTransitionToFailed(t *testing.T) {
	phases := []v1alpha1.VMPhase{
		v1alpha1.VMPhasePending,
		v1alpha1.VMPhaseCreating,
		v1alpha1.VMPhaseRunning,
		v1alpha1.VMPhaseStopping,
		v1alpha1.VMPhaseStopped,
	}

	for _, phase := range phases {
		t.Run(string(phase), func(t *testing.T) {
			vm := v1alpha1.NewVirtualMachine("test-vm")
			vm.SetPhase(phase)

			TransitionToFailed(vm, "TestFailure", "Test error message")

			if vm.GetPhase() != v1alpha1.VMPhaseFailed {
				t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
			}

			if !IsConditionFalse(vm, v1alpha1.ConditionReady) {
				t.Error("Expected Ready condition to be False")
			}

			cond := GetCondition(vm, v1alpha1.ConditionReady)
			if cond.Reason != "TestFailure" {
				t.Errorf("Expected reason 'TestFailure', got %s", cond.Reason)
			}
			if cond.Message != "Test error message" {
				t.Errorf("Expected message 'Test error message', got %s", cond.Message)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		phase    v1alpha1.VMPhase
		expected bool
	}{
		{v1alpha1.VMPhasePending, false},
		{v1alpha1.VMPhaseCreating, false},
		{v1alpha1.VMPhaseRunning, false},
		{v1alpha1.VMPhaseStopping, false},
		{v1alpha1.VMPhaseStopped, true},
		{v1alpha1.VMPhaseFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := IsTerminal(tt.phase); got != tt.expected {
				t.Errorf("IsTerminal(%s) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	tests := []struct {
		phase    v1alpha1.VMPhase
		expected bool
	}{
		{v1alpha1.VMPhasePending, false},
		{v1alpha1.VMPhaseCreating, false},
		{v1alpha1.VMPhaseRunning, true},
		{v1alpha1.VMPhaseStopping, false},
		{v1alpha1.VMPhaseStopped, false},
		{v1alpha1.VMPhaseFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := IsRunning(tt.phase); got != tt.expected {
				t.Errorf("IsRunning(%s) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestIsTransitioning(t *testing.T) {
	tests := []struct {
		phase    v1alpha1.VMPhase
		expected bool
	}{
		{v1alpha1.VMPhasePending, false},
		{v1alpha1.VMPhaseCreating, true},
		{v1alpha1.VMPhaseRunning, false},
		{v1alpha1.VMPhaseStopping, true},
		{v1alpha1.VMPhaseStopped, false},
		{v1alpha1.VMPhaseFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := IsTransitioning(tt.phase); got != tt.expected {
				t.Errorf("IsTransitioning(%s) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestPhaseTransitionFlow(t *testing.T) {
	// Test complete lifecycle: Pending -> Creating -> Running -> Stopping -> Stopped
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Start in Pending
	if vm.GetPhase() != v1alpha1.VMPhasePending {
		t.Fatalf("Expected initial phase Pending, got %s", vm.GetPhase())
	}

	// Pending -> Creating
	if err := TransitionToCreating(vm); err != nil {
		t.Fatalf("Failed to transition to Creating: %v", err)
	}

	// Creating -> Running
	if err := TransitionToRunning(vm); err != nil {
		t.Fatalf("Failed to transition to Running: %v", err)
	}

	// Running -> Stopping
	if err := TransitionToStopping(vm); err != nil {
		t.Fatalf("Failed to transition to Stopping: %v", err)
	}

	// Stopping -> Stopped
	if err := TransitionToStopped(vm); err != nil {
		t.Fatalf("Failed to transition to Stopped: %v", err)
	}

	// Verify final state
	if vm.GetPhase() != v1alpha1.VMPhaseStopped {
		t.Errorf("Expected final phase Stopped, got %s", vm.GetPhase())
	}
}

func TestPhaseTransitionFailureFlow(t *testing.T) {
	// Test failure during creation: Pending -> Creating -> Failed
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Pending -> Creating
	if err := TransitionToCreating(vm); err != nil {
		t.Fatalf("Failed to transition to Creating: %v", err)
	}

	// Creating -> Failed
	TransitionToFailed(vm, "CreationFailed", "Failed to create VM")

	if vm.GetPhase() != v1alpha1.VMPhaseFailed {
		t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
	}

	// Once failed, cannot transition to Running
	if err := TransitionToRunning(vm); err == nil {
		t.Error("Expected error transitioning from Failed to Running")
	}
}
