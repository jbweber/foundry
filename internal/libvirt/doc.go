// Package libvirt provides a client wrapper for interacting with libvirt.
//
// This package wraps github.com/digitalocean/go-libvirt to provide:
//   - Connection management (connect, disconnect, ping)
//   - Domain XML generation from VirtualMachine specs
//   - Utility functions for libvirt operations
//
// The Client type provides a high-level interface for libvirt operations,
// while exposing the underlying *libvirt.Libvirt for packages that need
// direct access to the libvirt API.
//
// Connection Management:
//
// The package establishes connections to the local libvirt daemon via Unix socket:
//
//	client, err := libvirt.Connect()
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Check connection
//	if err := client.Ping(); err != nil {
//	    return err
//	}
//
// Domain XML Generation:
//
// The package generates libvirt domain XML from VirtualMachine specs:
//
//	vm := &v1alpha1.VirtualMachine{
//	    ObjectMeta: metav1.ObjectMeta{Name: "myvm"},
//	    Spec: v1alpha1.VirtualMachineSpec{
//	        VCPUs:      2,
//	        MemoryGiB:  4,
//	        BootDisk:   v1alpha1.BootDisk{SizeGB: 20, Image: "fedora-43.qcow2"},
//	    },
//	}
//
//	xml, err := libvirt.GenerateDomainXML(vm, "foundry-vms")
//	if err != nil {
//	    return err
//	}
//
//	// Define domain in libvirt
//	dom, err := client.Libvirt().DomainDefineXML(xml)
//	if err != nil {
//	    return err
//	}
//
// Consumer-Side Interfaces:
//
// This package does not define interfaces. Instead, consumers (internal/vm,
// internal/storage, internal/metadata) define their own LibvirtClient interfaces
// specifying only the operations they need. The *libvirt.Libvirt type satisfies
// these interfaces implicitly, enabling clean dependency injection.
//
// See internal/vm/interfaces.go, internal/storage/types.go, and
// internal/metadata/storage.go for examples of consumer-side interfaces.
package libvirt
