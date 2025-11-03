package output

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// YAMLFormatter formats resources as YAML.
type YAMLFormatter struct{}

// FormatVM formats a single VirtualMachine as YAML.
func (f *YAMLFormatter) FormatVM(vm *v1alpha1.VirtualMachine) (string, error) {
	// Ensure TypeMeta is set
	v1alpha1.SetDefaultAPIVersion(vm)

	data, err := yaml.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM to YAML: %w", err)
	}

	return string(data), nil
}

// FormatVMList formats a list of VirtualMachines as YAML.
// Outputs as a YAML stream (multiple documents separated by ---).
func (f *YAMLFormatter) FormatVMList(vms []*v1alpha1.VirtualMachine) (string, error) {
	if len(vms) == 0 {
		return "", nil
	}

	var buf bytes.Buffer

	for i, vm := range vms {
		// Ensure TypeMeta is set
		v1alpha1.SetDefaultAPIVersion(vm)

		data, err := yaml.Marshal(vm)
		if err != nil {
			return "", fmt.Errorf("failed to marshal VM %s to YAML: %w", vm.Name, err)
		}

		// Add document separator between VMs (but not before the first one)
		if i > 0 {
			buf.WriteString("---\n")
		}

		buf.Write(data)
	}

	return buf.String(), nil
}
