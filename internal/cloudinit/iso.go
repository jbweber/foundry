// Package cloudinit provides cloud-init configuration generation for VM provisioning.
package cloudinit

import (
	"bytes"
	"fmt"

	"github.com/kdomanski/iso9660"

	"github.com/jbweber/foundry/api/v1alpha1"
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
func GenerateISO(vm *v1alpha1.VirtualMachine) ([]byte, error) {
	if vm == nil {
		return nil, fmt.Errorf("VM configuration cannot be nil")
	}

	// Generate the three cloud-init files
	userData, err := GenerateUserData(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user-data: %w", err)
	}

	metaData, err := GenerateMetaData(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to generate meta-data: %w", err)
	}

	networkConfig, err := GenerateNetworkConfig(vm)
	if err != nil {
		return nil, fmt.Errorf("failed to generate network-config: %w", err)
	}

	// The volume identifier "CIDATA" is required by the cloud-init NoCloud
	// datasource (uppercase, per the NoCloud specification)
	return BuildISO("CIDATA", []ISOFile{
		{Name: "user-data", Data: []byte(userData)},
		{Name: "meta-data", Data: []byte(metaData)},
		{Name: "network-config", Data: []byte(networkConfig)},
	})
}

// ISOFile is a file for the root directory of an ISO image.
type ISOFile struct {
	Name string
	Data []byte
}

// BuildISO builds an ISO-9660 image, in memory, with the given volume label
// and files, all in its root directory.
//
// The image is plain ISO-9660 (no Rock Ridge or Joliet): the writer
// lower-cases names and keeps only ISO-9660 d1-characters, and a Linux guest
// shows the names in lower case (isofs map=normal), so names should be short,
// lower-case and use at most one dot to reach the guest unchanged
// (https://github.com/kdomanski/iso9660).
func BuildISO(label string, files []ISOFile) ([]byte, error) {
	writer, err := iso9660.NewWriter()
	if err != nil {
		return nil, fmt.Errorf("failed to create ISO writer: %w", err)
	}
	defer func() {
		// The writer stages files in a temporary directory; a failure to
		// remove it does not affect the image, which is complete by then
		_ = writer.Cleanup()
	}()

	for _, f := range files {
		if err := writer.AddFile(bytes.NewReader(f.Data), f.Name); err != nil {
			return nil, fmt.Errorf("failed to add %s: %w", f.Name, err)
		}
	}

	var buf bytes.Buffer
	if err := writer.WriteTo(&buf, label); err != nil {
		return nil, fmt.Errorf("failed to write ISO image: %w", err)
	}
	return buf.Bytes(), nil
}
