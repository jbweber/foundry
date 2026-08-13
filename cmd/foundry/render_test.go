package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const renderTestConfig = `apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: render-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.20.30.40/24
      gateway: 10.20.30.1
      bridge: br0
      defaultRoute: true
      vlan: 100
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vm.yaml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	return path
}

func TestRenderDomainXML(t *testing.T) {
	xml, err := renderDomainXML(writeConfig(t, renderTestConfig))
	if err != nil {
		t.Fatalf("renderDomainXML() error = %v", err)
	}

	// Spot-check that spec details survive into the XML rather than asserting
	// the whole document; domain_test.go covers generation in detail.
	wants := []string{
		"<name>render-vm</name>",
		`<source bridge="br0">`,
		`<tag id="100">`,
	}
	for _, want := range wants {
		if !strings.Contains(xml, want) {
			t.Errorf("Rendered XML missing %q\nGot:\n%s", want, xml)
		}
	}
}

func TestRenderDomainXML_Errors(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(t.TempDir(), "does-not-exist.yaml")},
		{"malformed YAML", writeConfig(t, "this: is: not: valid")},
		{"fails validation", writeConfig(t, `apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: bad-vlan
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.20.30.40/24
      gateway: 10.20.30.1
      bridge: br0
      vlan: 4095
`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := renderDomainXML(tt.path); err == nil {
				t.Errorf("Expected error for %s", tt.name)
			}
		})
	}
}

// render must not require libvirt, so the command has to succeed in a test
// environment with no libvirtd socket.
func TestRenderCmd_WritesXMLToStdout(t *testing.T) {
	var stdout bytes.Buffer
	renderCmd.SetOut(&stdout)
	t.Cleanup(func() { renderCmd.SetOut(nil) })

	if err := renderCmd.RunE(renderCmd, []string{writeConfig(t, renderTestConfig)}); err != nil {
		t.Fatalf("render command error = %v", err)
	}

	out := stdout.String()
	if !strings.HasPrefix(out, `<domain type="kvm">`) {
		t.Errorf("Expected XML on stdout, got: %s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("Expected trailing newline after XML")
	}
}
