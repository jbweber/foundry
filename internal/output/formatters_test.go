package output

import (
	"strings"
	"testing"
	"time"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// createTestVM creates a VirtualMachine for testing.
func createTestVM(name string, phase v1alpha1.VMPhase, ip string) *v1alpha1.VirtualMachine {
	vm := &v1alpha1.VirtualMachine{
		TypeMeta: v1alpha1.TypeMeta{
			APIVersion: "foundry.cofront.xyz/v1alpha1",
			Kind:       "VirtualMachine",
		},
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: name,
			CreationTimestamp: v1alpha1.Time{
				Time: time.Now().Add(-5 * time.Minute),
			},
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
		},
		Status: v1alpha1.VirtualMachineStatus{
			Phase: phase,
		},
	}

	if ip != "" {
		vm.Status.Addresses = []v1alpha1.VMAddress{
			{
				Type:    "InternalIP",
				Address: ip,
			},
		}
	}

	return vm
}

func TestTableFormatter_FormatVM(t *testing.T) {
	tests := []struct {
		name      string
		vm        *v1alpha1.VirtualMachine
		wantName  string
		wantPhase string
	}{
		{
			name:      "running VM with IP",
			vm:        createTestVM("test-vm", v1alpha1.VMPhaseRunning, "10.0.0.1"),
			wantName:  "test-vm",
			wantPhase: "Running",
		},
		{
			name:      "stopped VM without IP",
			vm:        createTestVM("stopped-vm", v1alpha1.VMPhaseStopped, ""),
			wantName:  "stopped-vm",
			wantPhase: "Stopped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &TableFormatter{}
			output, err := formatter.FormatVM(tt.vm)
			if err != nil {
				t.Fatalf("FormatVM() error = %v", err)
			}

			// Check that output contains expected values
			if !strings.Contains(output, tt.wantName) {
				t.Errorf("output missing VM name %q: %s", tt.wantName, output)
			}
			if !strings.Contains(output, tt.wantPhase) {
				t.Errorf("output missing phase %q: %s", tt.wantPhase, output)
			}
		})
	}
}

func TestTableFormatter_FormatVMList(t *testing.T) {
	tests := []struct {
		name       string
		vms        []*v1alpha1.VirtualMachine
		noHeaders  bool
		wantCount  int
		wantHeader bool
	}{
		{
			name:      "empty list",
			vms:       []*v1alpha1.VirtualMachine{},
			wantCount: 0,
		},
		{
			name: "single VM",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
			},
			wantCount:  1,
			wantHeader: true,
		},
		{
			name: "multiple VMs",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
				createTestVM("vm2", v1alpha1.VMPhaseStopped, ""),
				createTestVM("vm3", v1alpha1.VMPhaseCreating, ""),
			},
			wantCount:  3,
			wantHeader: true,
		},
		{
			name: "no headers",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
			},
			noHeaders:  true,
			wantCount:  1,
			wantHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &TableFormatter{NoHeaders: tt.noHeaders}
			output, err := formatter.FormatVMList(tt.vms)
			if err != nil {
				t.Fatalf("FormatVMList() error = %v", err)
			}

			if tt.wantCount == 0 {
				if !strings.Contains(output, "No VMs found") {
					t.Errorf("expected 'No VMs found' message, got: %s", output)
				}
				return
			}

			// Check header presence
			hasHeader := strings.Contains(output, "NAME") && strings.Contains(output, "PHASE")
			if tt.wantHeader && !hasHeader {
				t.Errorf("expected header in output, got: %s", output)
			}
			if !tt.wantHeader && hasHeader {
				t.Errorf("expected no header in output, got: %s", output)
			}

			// Count lines (should be header + VM count, or just VM count if no headers)
			lines := strings.Split(strings.TrimSpace(output), "\n")
			expectedLines := tt.wantCount
			if tt.wantHeader {
				expectedLines++ // Add 1 for header
			}
			if len(lines) != expectedLines {
				t.Errorf("expected %d lines, got %d: %s", expectedLines, len(lines), output)
			}
		})
	}
}

func TestYAMLFormatter_FormatVM(t *testing.T) {
	vm := createTestVM("test-vm", v1alpha1.VMPhaseRunning, "10.0.0.1")

	formatter := &YAMLFormatter{}
	output, err := formatter.FormatVM(vm)
	if err != nil {
		t.Fatalf("FormatVM() error = %v", err)
	}

	// Check that output contains YAML structure
	requiredFields := []string{
		"apiVersion:",
		"kind:",
		"metadata:",
		"name: test-vm",
		"spec:",
		"vcpus: 2",
		"memoryGiB: 4",
		"status:",
		"phase: Running",
	}

	for _, field := range requiredFields {
		if !strings.Contains(output, field) {
			t.Errorf("output missing required field %q: %s", field, output)
		}
	}
}

func TestYAMLFormatter_FormatVMList(t *testing.T) {
	tests := []struct {
		name      string
		vms       []*v1alpha1.VirtualMachine
		wantEmpty bool
	}{
		{
			name:      "empty list",
			vms:       []*v1alpha1.VirtualMachine{},
			wantEmpty: true,
		},
		{
			name: "single VM",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
			},
		},
		{
			name: "multiple VMs",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
				createTestVM("vm2", v1alpha1.VMPhaseStopped, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &YAMLFormatter{}
			output, err := formatter.FormatVMList(tt.vms)
			if err != nil {
				t.Fatalf("FormatVMList() error = %v", err)
			}

			if tt.wantEmpty {
				if output != "" {
					t.Errorf("expected empty output, got: %s", output)
				}
				return
			}

			// For multiple VMs, check for document separator
			if len(tt.vms) > 1 {
				if !strings.Contains(output, "---") {
					t.Errorf("expected document separator '---' in output")
				}
			}

			// Check that all VM names appear
			for _, vm := range tt.vms {
				if !strings.Contains(output, vm.Name) {
					t.Errorf("output missing VM name %q", vm.Name)
				}
			}
		})
	}
}

func TestJSONFormatter_FormatVM(t *testing.T) {
	vm := createTestVM("test-vm", v1alpha1.VMPhaseRunning, "10.0.0.1")

	formatter := &JSONFormatter{}
	output, err := formatter.FormatVM(vm)
	if err != nil {
		t.Fatalf("FormatVM() error = %v", err)
	}

	// Check that output contains JSON structure
	requiredFields := []string{
		`"apiVersion"`,
		`"kind"`,
		`"metadata"`,
		`"name": "test-vm"`,
		`"spec"`,
		`"vcpus": 2`,
		`"memoryGiB": 4`,
		`"status"`,
		`"phase": "Running"`,
	}

	for _, field := range requiredFields {
		if !strings.Contains(output, field) {
			t.Errorf("output missing required field %q: %s", field, output)
		}
	}
}

func TestJSONFormatter_FormatVMList(t *testing.T) {
	tests := []struct {
		name      string
		vms       []*v1alpha1.VirtualMachine
		wantEmpty bool
	}{
		{
			name:      "empty list",
			vms:       []*v1alpha1.VirtualMachine{},
			wantEmpty: true,
		},
		{
			name: "single VM",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
			},
		},
		{
			name: "multiple VMs",
			vms: []*v1alpha1.VirtualMachine{
				createTestVM("vm1", v1alpha1.VMPhaseRunning, "10.0.0.1"),
				createTestVM("vm2", v1alpha1.VMPhaseStopped, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &JSONFormatter{}
			output, err := formatter.FormatVMList(tt.vms)
			if err != nil {
				t.Fatalf("FormatVMList() error = %v", err)
			}

			if tt.wantEmpty {
				expected := "[]\n"
				if output != expected {
					t.Errorf("expected %q, got: %q", expected, output)
				}
				return
			}

			// Check for array structure
			if !strings.HasPrefix(strings.TrimSpace(output), "[") {
				t.Errorf("expected output to start with '[': %s", output)
			}

			// Check that all VM names appear
			for _, vm := range tt.vms {
				if !strings.Contains(output, vm.Name) {
					t.Errorf("output missing VM name %q", vm.Name)
				}
			}
		})
	}
}

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name: "table format",
			opts: Options{Format: FormatTable},
		},
		{
			name: "yaml format",
			opts: Options{Format: FormatYAML},
		},
		{
			name: "json format",
			opts: Options{Format: FormatJSON},
		},
		{
			name:    "invalid format",
			opts:    Options{Format: "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter, err := NewFormatter(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFormatter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && formatter == nil {
				t.Error("NewFormatter() returned nil formatter")
			}
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:   "valid table",
			format: "table",
		},
		{
			name:   "valid yaml",
			format: "yaml",
		},
		{
			name:   "valid json",
			format: "json",
		},
		{
			name:    "invalid format",
			format:  "xml",
			wantErr: true,
		},
		{
			name:    "empty format",
			format:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"5 seconds", 5 * time.Second, "5s"},
		{"30 seconds", 30 * time.Second, "30s"},
		{"2 minutes", 2 * time.Minute, "2m"},
		{"90 seconds", 90 * time.Second, "1m"},
		{"2 hours", 2 * time.Hour, "2h"},
		{"90 minutes", 90 * time.Minute, "1h"},
		{"2 days", 48 * time.Hour, "2d"},
		{"2 weeks", 14 * 24 * time.Hour, "2w"},
		{"50 days", 50 * 24 * time.Hour, "7w"},
		{"60 days", 60 * 24 * time.Hour, "60d"}, // >= 8 weeks shows as days
		{"400 days", 400 * 24 * time.Hour, "1y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAge(tt.duration)
			if got != tt.want {
				t.Errorf("formatAge(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}
