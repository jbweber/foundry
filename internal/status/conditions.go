// Package status provides utilities for managing VirtualMachine status fields,
// including conditions and phase transitions.
package status

import (
	"time"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// SetCondition adds or updates a condition in the VM status.
// If a condition with the same type already exists, it updates it.
// The LastTransitionTime is only updated if the status changes.
func SetCondition(vm *v1alpha1.VirtualMachine, condType string, status v1alpha1.ConditionStatus, reason, message string) {
	now := v1alpha1.Time{Time: time.Now()}

	newCondition := v1alpha1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: vm.Generation,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == condType {
			// Update existing condition
			existing := &vm.Status.Conditions[i]

			// Only update LastTransitionTime if status changed
			if existing.Status != status {
				existing.LastTransitionTime = now
			}

			existing.Status = status
			existing.Reason = reason
			existing.Message = message
			existing.ObservedGeneration = vm.Generation
			return
		}
	}

	// Condition doesn't exist, append it
	vm.Status.Conditions = append(vm.Status.Conditions, newCondition)
}

// GetCondition returns a condition by type, or nil if not found.
func GetCondition(vm *v1alpha1.VirtualMachine, condType string) *v1alpha1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == condType {
			return &vm.Status.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue returns true if the condition exists and has status True.
func IsConditionTrue(vm *v1alpha1.VirtualMachine, condType string) bool {
	cond := GetCondition(vm, condType)
	return cond != nil && cond.Status == v1alpha1.ConditionTrue
}

// IsConditionFalse returns true if the condition exists and has status False.
func IsConditionFalse(vm *v1alpha1.VirtualMachine, condType string) bool {
	cond := GetCondition(vm, condType)
	return cond != nil && cond.Status == v1alpha1.ConditionFalse
}

// RemoveCondition removes a condition by type.
func RemoveCondition(vm *v1alpha1.VirtualMachine, condType string) {
	filtered := make([]v1alpha1.Condition, 0, len(vm.Status.Conditions))
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type != condType {
			filtered = append(filtered, vm.Status.Conditions[i])
		}
	}
	vm.Status.Conditions = filtered
}

// MarkReady sets all conditions to True and phase to Running.
// This is a convenience function for when a VM is fully operational.
func MarkReady(vm *v1alpha1.VirtualMachine) {
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionTrue, "VMReady", "VM is running and accessible")
	SetCondition(vm, v1alpha1.ConditionStorageProvisioned, v1alpha1.ConditionTrue, "StorageReady", "All storage volumes created successfully")
	SetCondition(vm, v1alpha1.ConditionNetworkConfigured, v1alpha1.ConditionTrue, "NetworkReady", "Network interfaces configured")
	SetCondition(vm, v1alpha1.ConditionCloudInitReady, v1alpha1.ConditionTrue, "CloudInitReady", "Cloud-init ISO created and attached")
	vm.SetPhase(v1alpha1.VMPhaseRunning)
	vm.UpdateObservedGeneration()
}

// MarkStorageProvisioned marks the storage provisioning condition as True.
func MarkStorageProvisioned(vm *v1alpha1.VirtualMachine) {
	SetCondition(vm, v1alpha1.ConditionStorageProvisioned, v1alpha1.ConditionTrue, "StorageCreated", "All storage volumes created successfully")
}

// MarkStorageFailed marks the storage provisioning condition as False.
func MarkStorageFailed(vm *v1alpha1.VirtualMachine, err error) {
	SetCondition(vm, v1alpha1.ConditionStorageProvisioned, v1alpha1.ConditionFalse, "StorageFailed", err.Error())
	vm.SetPhase(v1alpha1.VMPhaseFailed)
}

// MarkNetworkConfigured marks the network configuration condition as True.
func MarkNetworkConfigured(vm *v1alpha1.VirtualMachine) {
	SetCondition(vm, v1alpha1.ConditionNetworkConfigured, v1alpha1.ConditionTrue, "NetworkReady", "Network interfaces configured")
}

// MarkNetworkFailed marks the network configuration condition as False.
func MarkNetworkFailed(vm *v1alpha1.VirtualMachine, err error) {
	SetCondition(vm, v1alpha1.ConditionNetworkConfigured, v1alpha1.ConditionFalse, "NetworkFailed", err.Error())
	vm.SetPhase(v1alpha1.VMPhaseFailed)
}

// MarkCloudInitReady marks the cloud-init condition as True.
func MarkCloudInitReady(vm *v1alpha1.VirtualMachine) {
	SetCondition(vm, v1alpha1.ConditionCloudInitReady, v1alpha1.ConditionTrue, "CloudInitGenerated", "Cloud-init ISO created and attached")
}

// MarkCloudInitFailed marks the cloud-init condition as False.
func MarkCloudInitFailed(vm *v1alpha1.VirtualMachine, err error) {
	SetCondition(vm, v1alpha1.ConditionCloudInitReady, v1alpha1.ConditionFalse, "CloudInitFailed", err.Error())
	vm.SetPhase(v1alpha1.VMPhaseFailed)
}

// MarkFailed sets the Ready condition to False and phase to Failed.
func MarkFailed(vm *v1alpha1.VirtualMachine, reason, message string) {
	SetCondition(vm, v1alpha1.ConditionReady, v1alpha1.ConditionFalse, reason, message)
	vm.SetPhase(v1alpha1.VMPhaseFailed)
}
