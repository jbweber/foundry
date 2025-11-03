// Package metadata provides storage for VirtualMachine specifications using
// libvirt's custom XML metadata feature. This allows the spec to persist with
// the VM domain itself, eliminating the need for external storage.
package metadata

import (
	"encoding/xml"
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"gopkg.in/yaml.v3"

	"github.com/jbweber/foundry/api/v1alpha1"
)

const (
	// MetadataNamespace is the XML namespace for Foundry metadata.
	// This follows the pattern used by Kubernetes and other tools.
	MetadataNamespace = "http://foundry.cofront.xyz/v1alpha1"

	// MetadataKey is the key used to store/retrieve metadata from libvirt.
	MetadataKey = "foundry-vm-spec"
)

// FoundryMetadata is the XML structure for storing VirtualMachine data in libvirt.
// The spec is stored as YAML text for easy human readability when inspecting
// the domain XML directly.
type FoundryMetadata struct {
	XMLName xml.Name `xml:"metadata"`
	Xmlns   string   `xml:"xmlns,attr"`
	// SpecYAML contains the VirtualMachine spec serialized as YAML
	SpecYAML string `xml:",innerxml"`
}

// Store saves the VirtualMachine spec to libvirt domain metadata.
// This allows the spec to persist with the VM itself.
func Store(l *libvirt.Libvirt, domain libvirt.Domain, vm *v1alpha1.VirtualMachine) error {
	// Serialize the entire VirtualMachine (including TypeMeta, ObjectMeta, Spec) to YAML
	yamlData, err := yaml.Marshal(vm)
	if err != nil {
		return fmt.Errorf("failed to marshal VM spec to YAML: %w", err)
	}

	// Wrap in XML structure
	metadata := FoundryMetadata{
		Xmlns:    MetadataNamespace,
		SpecYAML: string(yamlData),
	}

	// Marshal to XML
	xmlData, err := xml.MarshalIndent(metadata, "  ", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata to XML: %w", err)
	}

	// Set metadata on domain
	// flags: 0 = replace existing metadata
	err = l.DomainSetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement), // Type: custom XML element
		libvirt.OptString{string(xmlData)},
		libvirt.OptString{MetadataKey}, // Key for our metadata
		libvirt.OptString{MetadataNamespace},
		libvirt.DomainModificationImpact(0), // flags: replace
	)
	if err != nil {
		return fmt.Errorf("failed to set libvirt domain metadata: %w", err)
	}

	return nil
}

// Load retrieves the VirtualMachine spec from libvirt domain metadata.
// Returns the full VirtualMachine object with spec populated.
func Load(l *libvirt.Libvirt, domain libvirt.Domain) (*v1alpha1.VirtualMachine, error) {
	// Get metadata from domain
	xmlStr, err := l.DomainGetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{MetadataNamespace},
		libvirt.DomainModificationImpact(0), // flags
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get libvirt domain metadata: %w", err)
	}

	// Parse XML
	var metadata FoundryMetadata
	if err := xml.Unmarshal([]byte(xmlStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata XML: %w", err)
	}

	// Parse YAML
	var vm v1alpha1.VirtualMachine
	if err := yaml.Unmarshal([]byte(metadata.SpecYAML), &vm); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VM spec from YAML: %w", err)
	}

	return &vm, nil
}

// Update updates the stored metadata for an existing VM.
// This is useful when the spec changes (e.g., after editing).
func Update(l *libvirt.Libvirt, domain libvirt.Domain, vm *v1alpha1.VirtualMachine) error {
	// Increment generation to track changes
	vm.Generation++

	return Store(l, domain, vm)
}

// Delete removes Foundry metadata from a domain.
// This is typically called during VM destruction cleanup.
func Delete(l *libvirt.Libvirt, domain libvirt.Domain) error {
	// Setting empty string with flags=1 removes the metadata
	err := l.DomainSetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{""}, // empty string removes metadata
		libvirt.OptString{MetadataKey},
		libvirt.OptString{MetadataNamespace},
		libvirt.DomainModificationImpact(1), // flags: remove
	)
	if err != nil {
		// Ignore "not found" errors - metadata may not exist
		return fmt.Errorf("failed to delete libvirt domain metadata: %w", err)
	}

	return nil
}

// Exists checks if Foundry metadata exists for a domain.
func Exists(l *libvirt.Libvirt, domain libvirt.Domain) bool {
	_, err := l.DomainGetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{MetadataNamespace},
		libvirt.DomainModificationImpact(0),
	)
	return err == nil
}
