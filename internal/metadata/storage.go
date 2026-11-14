// Package metadata provides storage for VirtualMachine specifications using
// libvirt's custom XML metadata feature. This allows the spec to persist with
// the VM domain itself, eliminating the need for external storage.
package metadata

import (
	"encoding/xml"
	"errors"
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"go.yaml.in/yaml/v3"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// LibvirtClient defines the minimal libvirt operations needed for metadata storage.
//
// This interface is defined on the consumer side (this package) following Go best practices
// for interface design. The concrete implementation is *libvirt.Libvirt from
// github.com/digitalocean/go-libvirt, which satisfies this interface implicitly.
//
// This pattern enables clean dependency injection:
//   - Production: Pass *libvirt.Libvirt to NewClient()
//   - Testing: Pass a mock implementation to NewClient()
//
// Example usage in production:
//
//	client, err := libvirt.Connect()
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	metaClient := metadata.NewClient(client.Libvirt())
//
// Example usage in tests:
//
//	mock := &MockLibvirtClient{...}
//	metaClient := metadata.NewClient(mock)
//
// This interface only includes domain metadata operations, following the
// Interface Segregation Principle.
type LibvirtClient interface {
	// DomainSetMetadata sets custom metadata on a domain
	DomainSetMetadata(dom libvirt.Domain, typ int32, metadata libvirt.OptString, key libvirt.OptString, uri libvirt.OptString, flags libvirt.DomainModificationImpact) error

	// DomainGetMetadata retrieves custom metadata from a domain
	DomainGetMetadata(dom libvirt.Domain, typ int32, uri libvirt.OptString, flags libvirt.DomainModificationImpact) (string, error)
}

// Client provides operations for storing and retrieving VirtualMachine metadata
// in libvirt domain metadata.
type Client struct {
	client LibvirtClient
}

// NewClient creates a new metadata client.
// Accepts any type implementing LibvirtClient (both *libvirt.Libvirt and test mocks).
func NewClient(client LibvirtClient) *Client {
	return &Client{
		client: client,
	}
}

const (
	// MetadataNamespace is the XML namespace for Foundry metadata.
	// This follows the pattern used by Kubernetes and other tools.
	MetadataNamespace = "http://foundry.cofront.xyz/v1alpha1"

	// MetadataKey is the key used to store/retrieve metadata from libvirt.
	MetadataKey = "foundry-vm-spec"

	// affectConfig is VIR_DOMAIN_AFFECT_CONFIG: the spec is kept in, and read
	// from, the domain's persistent definition, the one that survives a stop
	// (https://libvirt.org/html/libvirt-libvirt-domain.html#virDomainSetMetadata).
	// Not "current" (0): on a running domain that is the live definition,
	// which libvirt discards when the domain stops.
	affectConfig = libvirt.DomainAffectConfig
)

// FoundryMetadata is the XML structure for storing VirtualMachine data in libvirt.
// The spec is stored as YAML text for easy human readability when inspecting
// the domain XML directly.
type FoundryMetadata struct {
	XMLName xml.Name `xml:"metadata"`
	Xmlns   string   `xml:"xmlns,attr"`
	// SpecYAML contains the VirtualMachine spec serialized as YAML, stored as
	// XML text: escaped on Store and unescaped on Load, so a spec whose
	// strings hold "<" or "&" (a shell script, an inline file) makes valid
	// metadata, and the text libvirt hands back, which it re-serializes with
	// its own escaping, reads back unchanged.
	SpecYAML string `xml:",chardata"`
}

// MaxRecordBytes bounds the stored spec record, the XML Encode returns.
// libvirt's RPC protocol carries a string of at most 4 MiB (REMOTE_STRING_MAX,
// https://gitlab.com/libvirt/libvirt/-/blob/v11.3.0/src/remote/remote_protocol.x):
// a larger record fails DomainSetMetadata. Once stored, the record is also
// part of the domain XML that DomainGetXMLDesc returns as one string, next to
// the definition itself, which holds the spec's fw_cfg strings a second time;
// a domain whose XML no longer fits in one RPC string can't be read back, so
// list and destroy fail on it. 1.5 MiB for the record keeps the record and
// the definition together well under 4 MiB. It sits above the 1 MiB that
// input disk inline strings may total, so that limit still applies first.
const MaxRecordBytes = 3 << 19

// Encode returns the record Store writes for vm: its YAML as the text of the
// metadata element. It refuses a record larger than MaxRecordBytes, counted
// as stored (escaping included), so callers can check a spec before anything
// is created.
func Encode(vm *v1alpha1.VirtualMachine) (string, error) {
	// Serialize the entire VirtualMachine (including TypeMeta, ObjectMeta, Spec) to YAML
	yamlData, err := v1alpha1.EncodeYAML(vm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM spec to YAML: %w", err)
	}

	// Wrap in XML structure
	metadata := FoundryMetadata{
		Xmlns:    MetadataNamespace,
		SpecYAML: string(yamlData),
	}

	// Marshal to XML
	xmlData, err := xml.MarshalIndent(metadata, "  ", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata to XML: %w", err)
	}
	if len(xmlData) > MaxRecordBytes {
		return "", fmt.Errorf("the spec takes %d bytes as stored in the domain's metadata, at most %d (1.5 MiB) are allowed: "+
			"inline content (cloud-init user data, input disk and fw_cfg strings, annotations) is too large", len(xmlData), MaxRecordBytes)
	}
	return string(xmlData), nil
}

// Store saves the VirtualMachine spec to the domain's persistent definition,
// replacing any earlier value, so the spec stays with the VM across stops.
func (c *Client) Store(domain libvirt.Domain, vm *v1alpha1.VirtualMachine) error {
	record, err := Encode(vm)
	if err != nil {
		return err
	}

	err = c.client.DomainSetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement), // Type: custom XML element
		libvirt.OptString{record},
		libvirt.OptString{MetadataKey}, // Key for our metadata
		libvirt.OptString{MetadataNamespace},
		affectConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to set libvirt domain metadata: %w", err)
	}

	return nil
}

// IsNotFound reports whether err (from Load) means the domain has no foundry
// metadata: libvirt's "no domain metadata" error, as for a domain foundry did
// not make.
func IsNotFound(err error) bool {
	var le libvirt.Error
	return errors.As(err, &le) && le.Code == uint32(libvirt.ErrNoDomainMetadata)
}

// Load retrieves the VirtualMachine spec from the domain's persistent
// definition, where Store keeps it, whether the domain runs or not.
// Returns the full VirtualMachine object with spec populated.
func (c *Client) Load(domain libvirt.Domain) (*v1alpha1.VirtualMachine, error) {
	// Get metadata from domain
	xmlStr, err := c.client.DomainGetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{MetadataNamespace},
		affectConfig,
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
func (c *Client) Update(domain libvirt.Domain, vm *v1alpha1.VirtualMachine) error {
	// Increment generation to track changes
	vm.Generation++

	return c.Store(domain, vm)
}

// Delete removes Foundry metadata from a domain.
// This is typically called during VM destruction cleanup.
func (c *Client) Delete(domain libvirt.Domain) error {
	// Setting an empty string removes the metadata, from the persistent
	// definition where Store keeps it
	err := c.client.DomainSetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{""}, // empty string removes metadata
		libvirt.OptString{MetadataKey},
		libvirt.OptString{MetadataNamespace},
		affectConfig,
	)
	if err != nil {
		// Ignore "not found" errors - metadata may not exist
		return fmt.Errorf("failed to delete libvirt domain metadata: %w", err)
	}

	return nil
}

// Exists checks if Foundry metadata exists in a domain's persistent definition.
func (c *Client) Exists(domain libvirt.Domain) bool {
	_, err := c.client.DomainGetMetadata(
		domain,
		int32(libvirt.DomainMetadataElement),
		libvirt.OptString{MetadataNamespace},
		affectConfig,
	)
	return err == nil
}
