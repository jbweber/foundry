package output

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// TableFormatter formats resources as human-readable tables.
type TableFormatter struct {
	// NoHeaders omits the header row.
	NoHeaders bool
}

// FormatVM formats a single VirtualMachine as a table row.
func (f *TableFormatter) FormatVM(vm *v1alpha1.VirtualMachine) (string, error) {
	return f.FormatVMList([]*v1alpha1.VirtualMachine{vm})
}

// FormatVMList formats a list of VirtualMachines as a table.
func (f *TableFormatter) FormatVMList(vms []*v1alpha1.VirtualMachine) (string, error) {
	if len(vms) == 0 {
		return "No VMs found\n", nil
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// Write header unless NoHeaders is set
	if !f.NoHeaders {
		_, _ = fmt.Fprintln(w, "NAME\tPHASE\tIP\tVCPUs\tMEMORY\tAGE")
	}

	// Write each VM as a row
	for _, vm := range vms {
		name := vm.Name
		phase := string(vm.Status.Phase)
		if phase == "" {
			phase = "-"
		}

		// Get first IP address from status
		ip := "-"
		if len(vm.Status.Addresses) > 0 {
			ip = vm.Status.Addresses[0].Address
		}

		vcpus := fmt.Sprintf("%d", vm.Spec.VCPUs)
		memory := fmt.Sprintf("%d GiB", vm.Spec.MemoryGiB)

		// Calculate age from creation timestamp
		age := "-"
		if !vm.CreationTimestamp.IsZero() {
			age = formatAge(time.Since(vm.CreationTimestamp.Time))
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			name, phase, ip, vcpus, memory, age)
	}

	_ = w.Flush()
	return buf.String(), nil
}

// formatAge formats a duration as a human-readable age string.
// Examples: "5s", "2m", "3h", "4d", "2w", "1y"
func formatAge(d time.Duration) string {
	// Handle negative durations (shouldn't happen, but be defensive)
	if d < 0 {
		return "unknown"
	}

	seconds := int(d.Seconds())

	// Less than 1 minute
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}

	minutes := seconds / 60
	// Less than 1 hour
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}

	hours := minutes / 60
	// Less than 1 day
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}

	days := hours / 24
	// Less than 1 week
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}

	weeks := days / 7
	// Less than ~2 months (8 weeks)
	if weeks < 8 {
		return fmt.Sprintf("%dw", weeks)
	}

	// More than 2 months, show in approximate years/days
	years := days / 365
	if years > 0 {
		return fmt.Sprintf("%dy", years)
	}

	return fmt.Sprintf("%dd", days)
}
