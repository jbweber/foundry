package status

import (
	"errors"
	"testing"
	"time"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func TestSetCondition_NewCondition(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	vm.Generation = 5

	SetCondition(vm, "TestCondition", v1alpha1.ConditionTrue, "TestReason", "Test message")

	if len(vm.Status.Conditions) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(vm.Status.Conditions))
	}

	cond := vm.Status.Conditions[0]
	if cond.Type != "TestCondition" {
		t.Errorf("Expected Type 'TestCondition', got %s", cond.Type)
	}
	if cond.Status != v1alpha1.ConditionTrue {
		t.Errorf("Expected Status True, got %s", cond.Status)
	}
	if cond.Reason != "TestReason" {
		t.Errorf("Expected Reason 'TestReason', got %s", cond.Reason)
	}
	if cond.Message != "Test message" {
		t.Errorf("Expected Message 'Test message', got %s", cond.Message)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("Expected ObservedGeneration 5, got %d", cond.ObservedGeneration)
	}
	if cond.LastTransitionTime.IsZero() {
		t.Error("Expected LastTransitionTime to be set")
	}
}

func TestSetCondition_UpdateExisting(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	vm.Generation = 1

	// Set initial condition
	SetCondition(vm, "Ready", v1alpha1.ConditionFalse, "NotReady", "VM not ready")
	initialTime := vm.Status.Conditions[0].LastTransitionTime

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update with same status - should NOT update LastTransitionTime
	SetCondition(vm, "Ready", v1alpha1.ConditionFalse, "StillNotReady", "Still not ready")

	if len(vm.Status.Conditions) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(vm.Status.Conditions))
	}

	cond := vm.Status.Conditions[0]
	if cond.Reason != "StillNotReady" {
		t.Errorf("Expected updated reason 'StillNotReady', got %s", cond.Reason)
	}
	if !cond.LastTransitionTime.Equal(initialTime.Time) {
		t.Error("LastTransitionTime should not change when status doesn't change")
	}

	// Update with different status - should update LastTransitionTime
	time.Sleep(10 * time.Millisecond)
	SetCondition(vm, "Ready", v1alpha1.ConditionTrue, "NowReady", "VM is ready")

	if len(vm.Status.Conditions) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(vm.Status.Conditions))
	}

	cond = vm.Status.Conditions[0]
	if cond.Status != v1alpha1.ConditionTrue {
		t.Errorf("Expected Status True, got %s", cond.Status)
	}
	if cond.LastTransitionTime.Equal(initialTime.Time) {
		t.Error("LastTransitionTime should change when status changes")
	}
}

func TestGetCondition(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Empty conditions
	if cond := GetCondition(vm, "NonExistent"); cond != nil {
		t.Error("Expected nil for non-existent condition")
	}

	// Add some conditions
	SetCondition(vm, "Ready", v1alpha1.ConditionTrue, "Ready", "")
	SetCondition(vm, "StorageProvisioned", v1alpha1.ConditionTrue, "Provisioned", "")

	// Get existing condition
	cond := GetCondition(vm, "Ready")
	if cond == nil {
		t.Fatal("Expected to find Ready condition")
	}
	if cond.Type != "Ready" {
		t.Errorf("Expected Type 'Ready', got %s", cond.Type)
	}

	// Get non-existent condition
	if cond := GetCondition(vm, "NonExistent"); cond != nil {
		t.Error("Expected nil for non-existent condition")
	}
}

func TestIsConditionTrue(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Non-existent condition
	if IsConditionTrue(vm, "Ready") {
		t.Error("Expected false for non-existent condition")
	}

	// False condition
	SetCondition(vm, "Ready", v1alpha1.ConditionFalse, "NotReady", "")
	if IsConditionTrue(vm, "Ready") {
		t.Error("Expected false for False condition")
	}

	// True condition
	SetCondition(vm, "Ready", v1alpha1.ConditionTrue, "Ready", "")
	if !IsConditionTrue(vm, "Ready") {
		t.Error("Expected true for True condition")
	}

	// Unknown condition
	SetCondition(vm, "Ready", v1alpha1.ConditionUnknown, "Unknown", "")
	if IsConditionTrue(vm, "Ready") {
		t.Error("Expected false for Unknown condition")
	}
}

func TestIsConditionFalse(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Non-existent condition
	if IsConditionFalse(vm, "Ready") {
		t.Error("Expected false for non-existent condition")
	}

	// True condition
	SetCondition(vm, "Ready", v1alpha1.ConditionTrue, "Ready", "")
	if IsConditionFalse(vm, "Ready") {
		t.Error("Expected false for True condition")
	}

	// False condition
	SetCondition(vm, "Ready", v1alpha1.ConditionFalse, "NotReady", "")
	if !IsConditionFalse(vm, "Ready") {
		t.Error("Expected true for False condition")
	}

	// Unknown condition
	SetCondition(vm, "Ready", v1alpha1.ConditionUnknown, "Unknown", "")
	if IsConditionFalse(vm, "Ready") {
		t.Error("Expected false for Unknown condition")
	}
}

func TestRemoveCondition(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	// Remove from empty list
	RemoveCondition(vm, "NonExistent")
	if len(vm.Status.Conditions) != 0 {
		t.Error("Expected 0 conditions after removing from empty list")
	}

	// Add multiple conditions
	SetCondition(vm, "Ready", v1alpha1.ConditionTrue, "Ready", "")
	SetCondition(vm, "StorageProvisioned", v1alpha1.ConditionTrue, "Provisioned", "")
	SetCondition(vm, "NetworkConfigured", v1alpha1.ConditionTrue, "Configured", "")

	if len(vm.Status.Conditions) != 3 {
		t.Fatalf("Expected 3 conditions, got %d", len(vm.Status.Conditions))
	}

	// Remove middle condition
	RemoveCondition(vm, "StorageProvisioned")
	if len(vm.Status.Conditions) != 2 {
		t.Fatalf("Expected 2 conditions after removal, got %d", len(vm.Status.Conditions))
	}

	// Verify removed condition is gone
	if GetCondition(vm, "StorageProvisioned") != nil {
		t.Error("Expected StorageProvisioned to be removed")
	}

	// Verify other conditions still exist
	if GetCondition(vm, "Ready") == nil {
		t.Error("Expected Ready condition to still exist")
	}
	if GetCondition(vm, "NetworkConfigured") == nil {
		t.Error("Expected NetworkConfigured condition to still exist")
	}

	// Remove non-existent condition
	RemoveCondition(vm, "NonExistent")
	if len(vm.Status.Conditions) != 2 {
		t.Error("Removing non-existent condition should not affect list")
	}
}

func TestMarkReady(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	vm.Generation = 5

	MarkReady(vm)

	// Check phase
	if vm.GetPhase() != v1alpha1.VMPhaseRunning {
		t.Errorf("Expected phase Running, got %s", vm.GetPhase())
	}

	// Check ObservedGeneration
	if vm.Status.ObservedGeneration != 5 {
		t.Errorf("Expected ObservedGeneration 5, got %d", vm.Status.ObservedGeneration)
	}

	// Check all conditions are set to True
	expectedConditions := []string{
		v1alpha1.ConditionReady,
		v1alpha1.ConditionStorageProvisioned,
		v1alpha1.ConditionNetworkConfigured,
		v1alpha1.ConditionCloudInitReady,
	}

	if len(vm.Status.Conditions) != len(expectedConditions) {
		t.Errorf("Expected %d conditions, got %d", len(expectedConditions), len(vm.Status.Conditions))
	}

	for _, condType := range expectedConditions {
		if !IsConditionTrue(vm, condType) {
			t.Errorf("Expected condition %s to be True", condType)
		}
	}
}

func TestMarkStorageProvisioned(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	MarkStorageProvisioned(vm)

	if !IsConditionTrue(vm, v1alpha1.ConditionStorageProvisioned) {
		t.Error("Expected StorageProvisioned condition to be True")
	}

	cond := GetCondition(vm, v1alpha1.ConditionStorageProvisioned)
	if cond.Reason != "StorageCreated" {
		t.Errorf("Expected reason 'StorageCreated', got %s", cond.Reason)
	}
}

func TestMarkStorageFailed(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	testErr := errors.New("storage creation failed")

	MarkStorageFailed(vm, testErr)

	if !IsConditionFalse(vm, v1alpha1.ConditionStorageProvisioned) {
		t.Error("Expected StorageProvisioned condition to be False")
	}

	cond := GetCondition(vm, v1alpha1.ConditionStorageProvisioned)
	if cond.Reason != "StorageFailed" {
		t.Errorf("Expected reason 'StorageFailed', got %s", cond.Reason)
	}
	if cond.Message != testErr.Error() {
		t.Errorf("Expected message '%s', got %s", testErr.Error(), cond.Message)
	}

	if vm.GetPhase() != v1alpha1.VMPhaseFailed {
		t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
	}
}

func TestMarkNetworkConfigured(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	MarkNetworkConfigured(vm)

	if !IsConditionTrue(vm, v1alpha1.ConditionNetworkConfigured) {
		t.Error("Expected NetworkConfigured condition to be True")
	}

	cond := GetCondition(vm, v1alpha1.ConditionNetworkConfigured)
	if cond.Reason != "NetworkReady" {
		t.Errorf("Expected reason 'NetworkReady', got %s", cond.Reason)
	}
}

func TestMarkNetworkFailed(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	testErr := errors.New("network configuration failed")

	MarkNetworkFailed(vm, testErr)

	if !IsConditionFalse(vm, v1alpha1.ConditionNetworkConfigured) {
		t.Error("Expected NetworkConfigured condition to be False")
	}

	cond := GetCondition(vm, v1alpha1.ConditionNetworkConfigured)
	if cond.Reason != "NetworkFailed" {
		t.Errorf("Expected reason 'NetworkFailed', got %s", cond.Reason)
	}

	if vm.GetPhase() != v1alpha1.VMPhaseFailed {
		t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
	}
}

func TestMarkCloudInitReady(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	MarkCloudInitReady(vm)

	if !IsConditionTrue(vm, v1alpha1.ConditionCloudInitReady) {
		t.Error("Expected CloudInitReady condition to be True")
	}

	cond := GetCondition(vm, v1alpha1.ConditionCloudInitReady)
	if cond.Reason != "CloudInitGenerated" {
		t.Errorf("Expected reason 'CloudInitGenerated', got %s", cond.Reason)
	}
}

func TestMarkCloudInitFailed(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")
	testErr := errors.New("cloud-init generation failed")

	MarkCloudInitFailed(vm, testErr)

	if !IsConditionFalse(vm, v1alpha1.ConditionCloudInitReady) {
		t.Error("Expected CloudInitReady condition to be False")
	}

	cond := GetCondition(vm, v1alpha1.ConditionCloudInitReady)
	if cond.Reason != "CloudInitFailed" {
		t.Errorf("Expected reason 'CloudInitFailed', got %s", cond.Reason)
	}

	if vm.GetPhase() != v1alpha1.VMPhaseFailed {
		t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
	}
}

func TestMarkFailed(t *testing.T) {
	vm := v1alpha1.NewVirtualMachine("test-vm")

	MarkFailed(vm, "TestFailure", "Something went wrong")

	if !IsConditionFalse(vm, v1alpha1.ConditionReady) {
		t.Error("Expected Ready condition to be False")
	}

	cond := GetCondition(vm, v1alpha1.ConditionReady)
	if cond.Reason != "TestFailure" {
		t.Errorf("Expected reason 'TestFailure', got %s", cond.Reason)
	}
	if cond.Message != "Something went wrong" {
		t.Errorf("Expected message 'Something went wrong', got %s", cond.Message)
	}

	if vm.GetPhase() != v1alpha1.VMPhaseFailed {
		t.Errorf("Expected phase Failed, got %s", vm.GetPhase())
	}
}
