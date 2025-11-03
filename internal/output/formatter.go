// Package output provides formatters for displaying Foundry resources
// in various formats (table, YAML, JSON).
package output

import (
	"fmt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// Format represents an output format type.
type Format string

const (
	// FormatTable is a human-readable table format.
	FormatTable Format = "table"
	// FormatYAML is a YAML format for declarative configs.
	FormatYAML Format = "yaml"
	// FormatJSON is a JSON format for machine consumption.
	FormatJSON Format = "json"
)

// Formatter formats Foundry resources for output.
type Formatter interface {
	// FormatVM formats a single VirtualMachine resource.
	FormatVM(vm *v1alpha1.VirtualMachine) (string, error)

	// FormatVMList formats a list of VirtualMachine resources.
	FormatVMList(vms []*v1alpha1.VirtualMachine) (string, error)
}

// Options contains options for formatting output.
type Options struct {
	// Format specifies the output format.
	Format Format
	// NoHeaders omits headers in table format.
	NoHeaders bool
}

// NewFormatter creates a new Formatter based on the specified format.
func NewFormatter(opts Options) (Formatter, error) {
	switch opts.Format {
	case FormatTable:
		return &TableFormatter{NoHeaders: opts.NoHeaders}, nil
	case FormatYAML:
		return &YAMLFormatter{}, nil
	case FormatJSON:
		return &JSONFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported output format: %s (supported: table, yaml, json)", opts.Format)
	}
}

// ValidateFormat checks if a format string is valid.
func ValidateFormat(format string) error {
	f := Format(format)
	switch f {
	case FormatTable, FormatYAML, FormatJSON:
		return nil
	default:
		return fmt.Errorf("invalid format: %s (valid formats: table, yaml, json)", format)
	}
}
