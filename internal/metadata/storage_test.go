package metadata

import (
	"encoding/xml"
	"errors"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// mockLibvirtClient is a mock implementation of LibvirtClient for testing.
type mockLibvirtClient struct {
	// For controlling behavior
	setMetadataError error
	getMetadataError error
	getMetadataValue string

	// For verification
	lastSetMetadata  string
	lastSetKey       string
	lastSetURI       string
	lastSetFlags     libvirt.DomainModificationImpact
	setMetadataCalls int
	getMetadataCalls int
}

func (m *mockLibvirtClient) DomainSetMetadata(
	dom libvirt.Domain,
	typ int32,
	metadata libvirt.OptString,
	key libvirt.OptString,
	uri libvirt.OptString,
	flags libvirt.DomainModificationImpact,
) error {
	m.setMetadataCalls++
	if len(metadata) > 0 {
		m.lastSetMetadata = metadata[0]
	}
	if len(key) > 0 {
		m.lastSetKey = key[0]
	}
	if len(uri) > 0 {
		m.lastSetURI = uri[0]
	}
	m.lastSetFlags = flags

	return m.setMetadataError
}

func (m *mockLibvirtClient) DomainGetMetadata(
	dom libvirt.Domain,
	typ int32,
	uri libvirt.OptString,
	flags libvirt.DomainModificationImpact,
) (string, error) {
	m.getMetadataCalls++
	return m.getMetadataValue, m.getMetadataError
}

// Helper function to create a minimal valid VM for testing.
func newTestVM(name string) *v1alpha1.VirtualMachine {
	autostart := true
	return &v1alpha1.VirtualMachine{
		TypeMeta: v1alpha1.TypeMeta{
			Kind:       "VirtualMachine",
			APIVersion: "foundry.cofront.xyz/v1alpha1",
		},
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 20,
				Image:  "fedora-43.qcow2",
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:           "10.250.250.10/24",
					Gateway:      "10.250.250.1",
					Bridge:       "br0",
					DNSServers:   []string{"8.8.8.8"},
					DefaultRoute: true,
				},
			},
			Autostart: &autostart,
		},
	}
}

// Helper function to create a VM with all optional fields populated.
func newCompleteTestVM(name string) *v1alpha1.VirtualMachine {
	autostart := true
	vm := newTestVM(name)
	vm.Labels = map[string]string{"env": "test"}
	vm.Annotations = map[string]string{"note": "test-vm"}
	vm.Spec.CPUMode = "host-passthrough"
	vm.Spec.StoragePool = "custom-pool"
	vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{
		{Device: "vdb", SizeGB: 50},
	}
	vm.Spec.CloudInit = &v1alpha1.CloudInitSpec{
		FQDN:              "test.example.com",
		SSHAuthorizedKeys: []string{"ssh-rsa AAAA..."},
		PasswordHash:      "$6$rounds=4096$...",
		SSHPasswordAuth:   false,
	}
	vm.Spec.Autostart = &autostart
	return vm
}

func TestStore_ValidVM(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := newTestVM("test-vm")

	err := Store(mock, domain, vm)

	if err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}

	if mock.lastSetKey != MetadataKey {
		t.Errorf("Expected key %q, got %q", MetadataKey, mock.lastSetKey)
	}

	if mock.lastSetURI != MetadataNamespace {
		t.Errorf("Expected URI %q, got %q", MetadataNamespace, mock.lastSetURI)
	}

	if mock.lastSetFlags != 0 {
		t.Errorf("Expected flags 0 (replace), got %d", mock.lastSetFlags)
	}

	// Verify the XML can be parsed back
	var metadata FoundryMetadata
	if err := xml.Unmarshal([]byte(mock.lastSetMetadata), &metadata); err != nil {
		t.Fatalf("Failed to parse stored XML: %v", err)
	}

	if metadata.Xmlns != MetadataNamespace {
		t.Errorf("Expected xmlns %q, got %q", MetadataNamespace, metadata.Xmlns)
	}

	if metadata.SpecYAML == "" {
		t.Error("Expected non-empty YAML spec")
	}
}

func TestStore_CompleteVM(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := newCompleteTestVM("complete-vm")

	err := Store(mock, domain, vm)

	if err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Verify we can parse back the stored data
	var metadata FoundryMetadata
	if err := xml.Unmarshal([]byte(mock.lastSetMetadata), &metadata); err != nil {
		t.Fatalf("Failed to parse stored XML: %v", err)
	}

	// Verify the YAML contains expected fields
	if metadata.SpecYAML == "" {
		t.Error("Expected non-empty YAML spec")
	}
}

func TestStore_MinimalVM(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := &v1alpha1.VirtualMachine{
		TypeMeta: v1alpha1.TypeMeta{
			Kind:       "VirtualMachine",
			APIVersion: "foundry.cofront.xyz/v1alpha1",
		},
		ObjectMeta: v1alpha1.ObjectMeta{
			Name: "minimal",
		},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     1,
			MemoryGiB: 1,
			BootDisk: v1alpha1.BootDiskSpec{
				SizeGB: 10,
			},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{
					IP:      "10.0.0.1/24",
					Gateway: "10.0.0.254",
					Bridge:  "br0",
				},
			},
		},
	}

	err := Store(mock, domain, vm)

	if err != nil {
		t.Fatalf("Store() failed with minimal VM: %v", err)
	}

	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}
}

func TestStore_DomainSetMetadataError(t *testing.T) {
	mock := &mockLibvirtClient{
		setMetadataError: errors.New("libvirt error"),
	}
	domain := libvirt.Domain{}
	vm := newTestVM("test-vm")

	err := Store(mock, domain, vm)

	if err == nil {
		t.Fatal("Expected error from Store(), got nil")
	}

	if !errors.Is(err, mock.setMetadataError) {
		t.Errorf("Expected error to wrap libvirt error")
	}
}

func TestStore_EmptyVMName(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := newTestVM("")

	// Should not fail - empty name is still valid YAML
	err := Store(mock, domain, vm)

	if err != nil {
		t.Fatalf("Store() failed with empty name: %v", err)
	}
}

func TestStore_NilVM(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}

	// Go's yaml.Marshal handles nil gracefully (marshals to "null")
	// This test just ensures we don't panic with nil input
	err := Store(mock, domain, nil)

	if err != nil {
		t.Fatalf("Store() failed with nil VM: %v", err)
	}

	// Verify it was stored (even though it's just "null")
	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}
}

func TestLoad_ValidMetadata(t *testing.T) {
	// Create the XML that would be stored
	metadata := FoundryMetadata{
		Xmlns: MetadataNamespace,
		SpecYAML: `kind: VirtualMachine
apiVersion: foundry.cofront.xyz/v1alpha1
metadata:
  name: test-vm
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 20
    image: fedora-43.qcow2
  networkInterfaces:
  - ip: 10.250.250.10/24
    gateway: 10.250.250.1
    bridge: br0
    dnsServers:
    - 8.8.8.8
    defaultRoute: true
  autostart: true
`,
	}
	xmlData, _ := xml.MarshalIndent(metadata, "  ", "  ")

	mock := &mockLibvirtClient{
		getMetadataValue: string(xmlData),
	}
	domain := libvirt.Domain{}

	loadedVM, err := Load(mock, domain)

	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loadedVM == nil {
		t.Fatal("Expected non-nil VM from Load()")
	}

	if loadedVM.Name != "test-vm" {
		t.Errorf("Expected name 'test-vm', got %q", loadedVM.Name)
	}

	if loadedVM.Spec.VCPUs != 2 {
		t.Errorf("Expected 2 VCPUs, got %d", loadedVM.Spec.VCPUs)
	}

	if loadedVM.Spec.MemoryGiB != 4 {
		t.Errorf("Expected 4 GiB memory, got %d", loadedVM.Spec.MemoryGiB)
	}

	if mock.getMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainGetMetadata call, got %d", mock.getMetadataCalls)
	}
}

func TestLoad_CompleteVM(t *testing.T) {
	metadata := FoundryMetadata{
		Xmlns: MetadataNamespace,
		SpecYAML: `kind: VirtualMachine
apiVersion: foundry.cofront.xyz/v1alpha1
metadata:
  name: complete-vm
  labels:
    env: test
  annotations:
    note: test-vm
spec:
  vcpus: 4
  cpuMode: host-passthrough
  memoryGiB: 8
  storagePool: custom-pool
  bootDisk:
    sizeGB: 40
    image: ubuntu-22.04.qcow2
  dataDisks:
  - device: vdb
    sizeGB: 100
  networkInterfaces:
  - ip: 192.168.1.10/24
    gateway: 192.168.1.1
    bridge: br0
    dnsServers:
    - 8.8.8.8
    - 8.8.4.4
    defaultRoute: true
  cloudInit:
    fqdn: test.example.com
    sshAuthorizedKeys:
    - ssh-rsa AAAA...
    passwordHash: $6$rounds=4096$...
  autostart: true
`,
	}
	xmlData, _ := xml.MarshalIndent(metadata, "  ", "  ")

	mock := &mockLibvirtClient{
		getMetadataValue: string(xmlData),
	}
	domain := libvirt.Domain{}

	loadedVM, err := Load(mock, domain)

	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loadedVM.Name != "complete-vm" {
		t.Errorf("Expected name 'complete-vm', got %q", loadedVM.Name)
	}

	if len(loadedVM.Labels) != 1 {
		t.Errorf("Expected 1 label, got %d", len(loadedVM.Labels))
	}

	if len(loadedVM.Spec.DataDisks) != 1 {
		t.Errorf("Expected 1 data disk, got %d", len(loadedVM.Spec.DataDisks))
	}

	if loadedVM.Spec.CloudInit == nil {
		t.Error("Expected CloudInit config, got nil")
	}
}

func TestLoad_DomainGetMetadataError(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataError: errors.New("libvirt error"),
	}
	domain := libvirt.Domain{}

	vm, err := Load(mock, domain)

	if err == nil {
		t.Fatal("Expected error from Load(), got nil")
	}

	if vm != nil {
		t.Error("Expected nil VM on error")
	}
}

func TestLoad_InvalidXML(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataValue: "not valid xml",
	}
	domain := libvirt.Domain{}

	vm, err := Load(mock, domain)

	if err == nil {
		t.Fatal("Expected error from Load() with invalid XML, got nil")
	}

	if vm != nil {
		t.Error("Expected nil VM on XML parse error")
	}
}

func TestLoad_CorruptedXML(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataValue: `<metadata xmlns="wrong-namespace">corrupted</metadata>`,
	}
	domain := libvirt.Domain{}

	vm, err := Load(mock, domain)

	// Should succeed in parsing XML but fail on YAML unmarshal
	if err == nil {
		t.Fatal("Expected error from Load() with corrupted XML, got nil")
	}

	if vm != nil {
		t.Error("Expected nil VM on YAML parse error")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	metadata := FoundryMetadata{
		Xmlns:    MetadataNamespace,
		SpecYAML: "not: valid: yaml: [[[",
	}
	xmlData, _ := xml.MarshalIndent(metadata, "  ", "  ")

	mock := &mockLibvirtClient{
		getMetadataValue: string(xmlData),
	}
	domain := libvirt.Domain{}

	vm, err := Load(mock, domain)

	if err == nil {
		t.Fatal("Expected error from Load() with invalid YAML, got nil")
	}

	if vm != nil {
		t.Error("Expected nil VM on YAML parse error")
	}
}

func TestLoad_EmptyYAML(t *testing.T) {
	metadata := FoundryMetadata{
		Xmlns:    MetadataNamespace,
		SpecYAML: "",
	}
	xmlData, _ := xml.MarshalIndent(metadata, "  ", "  ")

	mock := &mockLibvirtClient{
		getMetadataValue: string(xmlData),
	}
	domain := libvirt.Domain{}

	// Empty YAML should parse to an empty VM struct
	vm, err := Load(mock, domain)

	if err != nil {
		t.Fatalf("Load() failed with empty YAML: %v", err)
	}

	if vm == nil {
		t.Fatal("Expected non-nil VM from Load()")
	}

	if vm.Name != "" {
		t.Error("Expected empty name for empty YAML")
	}
}

func TestUpdate_IncrementsGeneration(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := newTestVM("test-vm")
	vm.Generation = 1

	err := Update(mock, domain, vm)

	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	if vm.Generation != 2 {
		t.Errorf("Expected generation 2, got %d", vm.Generation)
	}

	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}
}

func TestUpdate_ModifiesExistingMetadata(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	vm := newTestVM("test-vm")
	vm.Generation = 5

	err := Update(mock, domain, vm)

	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	if vm.Generation != 6 {
		t.Errorf("Expected generation 6, got %d", vm.Generation)
	}
}

func TestUpdate_StoreError(t *testing.T) {
	mock := &mockLibvirtClient{
		setMetadataError: errors.New("libvirt error"),
	}
	domain := libvirt.Domain{}
	vm := newTestVM("test-vm")
	originalGeneration := vm.Generation

	err := Update(mock, domain, vm)

	if err == nil {
		t.Fatal("Expected error from Update(), got nil")
	}

	// Generation should still be incremented even though Store failed
	if vm.Generation != originalGeneration+1 {
		t.Errorf("Expected generation %d, got %d", originalGeneration+1, vm.Generation)
	}
}

func TestDelete_Success(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}

	err := Delete(mock, domain)

	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}

	if mock.lastSetMetadata != "" {
		t.Error("Expected empty string for delete operation")
	}

	if mock.lastSetKey != MetadataKey {
		t.Errorf("Expected key %q, got %q", MetadataKey, mock.lastSetKey)
	}

	if mock.lastSetURI != MetadataNamespace {
		t.Errorf("Expected URI %q, got %q", MetadataNamespace, mock.lastSetURI)
	}

	if mock.lastSetFlags != 1 {
		t.Errorf("Expected flags 1 (remove), got %d", mock.lastSetFlags)
	}
}

func TestDelete_NonExistentMetadata(t *testing.T) {
	// Even if metadata doesn't exist, Delete should still call DomainSetMetadata
	// The implementation doesn't check first, it just tries to delete
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}

	err := Delete(mock, domain)

	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	if mock.setMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainSetMetadata call, got %d", mock.setMetadataCalls)
	}
}

func TestDelete_Error(t *testing.T) {
	mock := &mockLibvirtClient{
		setMetadataError: errors.New("libvirt error"),
	}
	domain := libvirt.Domain{}

	err := Delete(mock, domain)

	if err == nil {
		t.Fatal("Expected error from Delete(), got nil")
	}
}

func TestExists_WithMetadata(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataValue: "<metadata>some data</metadata>",
	}
	domain := libvirt.Domain{}

	exists := Exists(mock, domain)

	if !exists {
		t.Error("Expected Exists() to return true when metadata exists")
	}

	if mock.getMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainGetMetadata call, got %d", mock.getMetadataCalls)
	}
}

func TestExists_WithoutMetadata(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataError: errors.New("metadata not found"),
	}
	domain := libvirt.Domain{}

	exists := Exists(mock, domain)

	if exists {
		t.Error("Expected Exists() to return false when metadata doesn't exist")
	}

	if mock.getMetadataCalls != 1 {
		t.Errorf("Expected 1 DomainGetMetadata call, got %d", mock.getMetadataCalls)
	}
}

func TestExists_LibvirtError(t *testing.T) {
	mock := &mockLibvirtClient{
		getMetadataError: errors.New("connection error"),
	}
	domain := libvirt.Domain{}

	exists := Exists(mock, domain)

	// Any error returns false
	if exists {
		t.Error("Expected Exists() to return false on error")
	}
}

func TestRoundTrip_StoreAndLoad(t *testing.T) {
	mock := &mockLibvirtClient{}
	domain := libvirt.Domain{}
	originalVM := newCompleteTestVM("roundtrip-vm")
	originalVM.Generation = 42

	// Store the VM
	err := Store(mock, domain, originalVM)
	if err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Set up mock to return what was stored
	mock.getMetadataValue = mock.lastSetMetadata

	// Load the VM back
	loadedVM, err := Load(mock, domain)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Compare key fields
	if loadedVM.Name != originalVM.Name {
		t.Errorf("Name mismatch: expected %q, got %q", originalVM.Name, loadedVM.Name)
	}

	if loadedVM.Spec.VCPUs != originalVM.Spec.VCPUs {
		t.Errorf("VCPUs mismatch: expected %d, got %d", originalVM.Spec.VCPUs, loadedVM.Spec.VCPUs)
	}

	if loadedVM.Spec.MemoryGiB != originalVM.Spec.MemoryGiB {
		t.Errorf("Memory mismatch: expected %d, got %d", originalVM.Spec.MemoryGiB, loadedVM.Spec.MemoryGiB)
	}

	if loadedVM.Generation != originalVM.Generation {
		t.Errorf("Generation mismatch: expected %d, got %d", originalVM.Generation, loadedVM.Generation)
	}

	if len(loadedVM.Spec.NetworkInterfaces) != len(originalVM.Spec.NetworkInterfaces) {
		t.Errorf("Network interfaces count mismatch: expected %d, got %d",
			len(originalVM.Spec.NetworkInterfaces), len(loadedVM.Spec.NetworkInterfaces))
	}
}

func TestMetadataConstants(t *testing.T) {
	// Verify constants haven't changed
	expectedNamespace := "http://foundry.cofront.xyz/v1alpha1"
	if MetadataNamespace != expectedNamespace {
		t.Errorf("MetadataNamespace changed: expected %q, got %q", expectedNamespace, MetadataNamespace)
	}

	expectedKey := "foundry-vm-spec"
	if MetadataKey != expectedKey {
		t.Errorf("MetadataKey changed: expected %q, got %q", expectedKey, MetadataKey)
	}
}
