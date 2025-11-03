package output

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// JSONFormatter formats resources as JSON.
type JSONFormatter struct{}

// FormatVM formats a single VirtualMachine as JSON.
func (f *JSONFormatter) FormatVM(vm *v1alpha1.VirtualMachine) (string, error) {
	// Ensure TypeMeta is set
	v1alpha1.SetDefaultAPIVersion(vm)

	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM to JSON: %w", err)
	}

	return string(data) + "\n", nil
}

// FormatVMList formats a list of VirtualMachines as JSON.
// Outputs as a JSON array.
func (f *JSONFormatter) FormatVMList(vms []*v1alpha1.VirtualMachine) (string, error) {
	if len(vms) == 0 {
		return "[]\n", nil
	}

	// Ensure TypeMeta is set for all VMs
	for _, vm := range vms {
		v1alpha1.SetDefaultAPIVersion(vm)
	}

	data, err := json.MarshalIndent(vms, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal VMs to JSON: %w", err)
	}

	return string(data) + "\n", nil
}

// FormatVMListAsItems formats a list of VirtualMachines as a JSON object with items array.
// This mimics Kubernetes List format:
//
//	{
//	  "apiVersion": "foundry.cofront.xyz/v1alpha1",
//	  "kind": "VirtualMachineList",
//	  "items": [...]
//	}
func (f *JSONFormatter) FormatVMListAsItems(vms []*v1alpha1.VirtualMachine) (string, error) {
	// Ensure TypeMeta is set for all VMs
	for _, vm := range vms {
		v1alpha1.SetDefaultAPIVersion(vm)
	}

	// Create a wrapper object
	wrapper := map[string]interface{}{
		"apiVersion": v1alpha1.GroupName + "/" + v1alpha1.Version,
		"kind":       "VirtualMachineList",
		"items":      vms,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(wrapper); err != nil {
		return "", fmt.Errorf("failed to marshal VM list to JSON: %w", err)
	}

	return buf.String(), nil
}
