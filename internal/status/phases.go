package status

import (
	"fmt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// TransitionToCreating transitions the VM phase to Creating.
// This should be called when starting VM creation.
func TransitionToCreating(vm *v1alpha1.VirtualMachine) error {
	// Can only transition from Pending to Creating
	if vm.GetPhase() != v1alpha1.VMPhasePending {
		return fmt.Errorf("cannot transition to Creating from phase %s", vm.GetPhase())
	}

	vm.SetPhase(v1alpha1.VMPhaseCreating)
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionFalse, "Creating", "VM creation in progress")
	return nil
}

// TransitionToRunning transitions the VM phase to Running.
// This should be called when VM creation completes successfully.
func TransitionToRunning(vm *v1alpha1.VirtualMachine) error {
	// Can transition from Creating or Stopped to Running
	phase := vm.GetPhase()
	if phase != v1alpha1.VMPhaseCreating && phase != v1alpha1.VMPhaseStopped {
		return fmt.Errorf("cannot transition to Running from phase %s", phase)
	}

	vm.SetPhase(v1alpha1.VMPhaseRunning)
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionTrue, "VMReady", "VM is running and accessible")
	vm.UpdateObservedGeneration()
	return nil
}

// TransitionToStopping transitions the VM phase to Stopping.
// This should be called when starting VM shutdown.
func TransitionToStopping(vm *v1alpha1.VirtualMachine) error {
	// Can only transition from Running to Stopping
	if vm.GetPhase() != v1alpha1.VMPhaseRunning {
		return fmt.Errorf("cannot transition to Stopping from phase %s", vm.GetPhase())
	}

	vm.SetPhase(v1alpha1.VMPhaseStopping)
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionFalse, "Stopping", "VM shutdown in progress")
	return nil
}

// TransitionToStopped transitions the VM phase to Stopped.
// This should be called when VM shutdown completes.
func TransitionToStopped(vm *v1alpha1.VirtualMachine) error {
	// Can transition from Stopping or Running (forced) to Stopped
	phase := vm.GetPhase()
	if phase != v1alpha1.VMPhaseStopping && phase != v1alpha1.VMPhaseRunning {
		return fmt.Errorf("cannot transition to Stopped from phase %s", phase)
	}

	vm.SetPhase(v1alpha1.VMPhaseStopped)
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionFalse, "Stopped", "VM has been stopped")
	return nil
}

// TransitionToFailed transitions the VM phase to Failed.
// This can happen from any phase when an error occurs.
func TransitionToFailed(vm *v1alpha1.VirtualMachine, reason, message string) {
	vm.SetPhase(v1alpha1.VMPhaseFailed)
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionFalse, reason, message)
}

// IsTerminal returns true if the phase is terminal (Stopped or Failed).
// Terminal phases mean the VM is not running and won't transition automatically.
func IsTerminal(phase v1alpha1.VMPhase) bool {
	return phase == v1alpha1.VMPhaseStopped || phase == v1alpha1.VMPhaseFailed
}

// IsRunning returns true if the VM is in a running state.
func IsRunning(phase v1alpha1.VMPhase) bool {
	return phase == v1alpha1.VMPhaseRunning
}

// IsTransitioning returns true if the VM is in a transitional state.
func IsTransitioning(phase v1alpha1.VMPhase) bool {
	return phase == v1alpha1.VMPhaseCreating || phase == v1alpha1.VMPhaseStopping
}
