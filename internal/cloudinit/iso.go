// Package cloudinit provides cloud-init configuration generation for VM provisioning.
package cloudinit

import (
	"bytes"
	"fmt"

	"github.com/kdomanski/iso9660"

	"github.com/jbweber/foundry/internal/config"
)

// GenerateISO creates a cloud-init NoCloud ISO image from the VM configuration.
//
// The generated ISO contains three files in the root directory:
//   - user-data: Cloud-config YAML with hostname, SSH keys, passwords
//   - meta-data: Instance metadata (instance-id, local-hostname)
//   - network-config: Netplan v2 network configuration
//
// The ISO volume label is set to "CIDATA" as required by the cloud-init NoCloud datasource.
//
// See https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html
//
// Returns the ISO image as a byte slice, ready to be uploaded to libvirt storage.
func GenerateISO(cfg *config.VMConfig) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("VM configuration cannot be nil")
	}

	// Generate the three cloud-init files
	userData, err := GenerateUserData(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user-data: %w", err)
	}

	metaData, err := GenerateMetaData(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate meta-data: %w", err)
	}

	networkConfig, err := GenerateNetworkConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate network-config: %w", err)
	}

	// Create a new ISO9660 image writer
	writer, err := iso9660.NewWriter()
	if err != nil {
		return nil, fmt.Errorf("failed to create ISO writer: %w", err)
	}
	defer func() {
		// Cleanup temporary files created by the ISO writer
		// Errors during cleanup are logged but don't fail the operation
		// since the ISO has already been generated
		_ = writer.Cleanup()
	}()

	// Add the three cloud-init files to the root directory
	// AddFile takes an io.Reader, so wrap the strings in bytes.NewReader
	if err := writer.AddFile(bytes.NewReader([]byte(userData)), "user-data"); err != nil {
		return nil, fmt.Errorf("failed to add user-data: %w", err)
	}

	if err := writer.AddFile(bytes.NewReader([]byte(metaData)), "meta-data"); err != nil {
		return nil, fmt.Errorf("failed to add meta-data: %w", err)
	}

	if err := writer.AddFile(bytes.NewReader([]byte(networkConfig)), "network-config"); err != nil {
		return nil, fmt.Errorf("failed to add network-config: %w", err)
	}

	// Create an in-memory buffer to hold the ISO image
	var buf bytes.Buffer

	// Write the ISO image to the buffer
	// The volume identifier "CIDATA" is passed here (required by cloud-init NoCloud datasource)
	// This must be uppercase per the NoCloud specification
	if err := writer.WriteTo(&buf, "CIDATA"); err != nil {
		return nil, fmt.Errorf("failed to write ISO image: %w", err)
	}

	return buf.Bytes(), nil
}
