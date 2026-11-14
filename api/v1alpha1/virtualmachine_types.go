package v1alpha1

// VirtualMachine represents a libvirt-based virtual machine managed by Foundry.
//
// This resource separates desired state (Spec) from observed state (Status),
// following Kubernetes API conventions. It can be used standalone via the
// Foundry CLI or as a Custom Resource Definition in a Kubernetes cluster.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vm;vms
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.status.addresses[0].address`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type VirtualMachine struct {
	// TypeMeta contains the API version and kind.
	TypeMeta `json:",inline" yaml:",inline"`

	// ObjectMeta contains metadata like name, labels, annotations.
	// +optional
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	// Spec defines the desired state of the VirtualMachine.
	Spec VirtualMachineSpec `json:"spec" yaml:"spec"`

	// Status defines the observed state of the VirtualMachine.
	// Populated by Foundry during VM lifecycle operations.
	// +optional
	Status VirtualMachineStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// VirtualMachineSpec defines the desired state of a VirtualMachine.
//
// +k8s:deepcopy-gen=true
type VirtualMachineSpec struct {
	// VCPUs is the number of virtual CPUs to allocate.
	// +kubebuilder:validation:Minimum=1
	VCPUs int `json:"vcpus" yaml:"vcpus"`

	// Arch is the guest CPU architecture, using libvirt's names.
	// Valid values: "x86_64" (default), "aarch64".
	// A guest of the hypervisor's own architecture runs under KVM; any other
	// runs under QEMU emulation (TCG), which is slow but needs no matching
	// hardware.
	// +optional
	// +kubebuilder:validation:Enum=x86_64;aarch64
	Arch string `json:"arch,omitempty" yaml:"arch,omitempty"`

	// CPUMode defines the CPU model exposure mode.
	// Valid values: "host-model", "host-passthrough", "maximum".
	// Defaults to "host-model" for an x86_64 guest under KVM,
	// "host-passthrough" for an aarch64 guest under KVM, and "maximum" for
	// an emulated guest, which cannot use the host's CPU.
	// +optional
	// +kubebuilder:validation:Enum=host-model;host-passthrough;maximum
	CPUMode string `json:"cpuMode,omitempty" yaml:"cpuMode,omitempty"`

	// MemoryGiB is the amount of memory to allocate in gibibytes (GiB).
	// +kubebuilder:validation:Minimum=1
	MemoryGiB int `json:"memoryGiB" yaml:"memoryGiB"`

	// StoragePool is the libvirt storage pool to use for VM disks.
	// Defaults to "foundry-vms" if not specified.
	// +optional
	// +kubebuilder:default=foundry-vms
	StoragePool string `json:"storagePool,omitempty" yaml:"storagePool,omitempty"`

	// BootDisk defines the primary boot disk configuration.
	BootDisk BootDiskSpec `json:"bootDisk" yaml:"bootDisk"`

	// DataDisks defines additional data disks to attach.
	// +optional
	DataDisks []DataDiskSpec `json:"dataDisks,omitempty" yaml:"dataDisks,omitempty"`

	// NetworkInterfaces defines the network interface configuration.
	// At least one interface is required.
	// +kubebuilder:validation:MinItems=1
	NetworkInterfaces []NetworkInterfaceSpec `json:"networkInterfaces" yaml:"networkInterfaces"`

	// CloudInit defines cloud-init configuration for VM provisioning.
	// +optional
	CloudInit *CloudInitSpec `json:"cloudInit,omitempty" yaml:"cloudInit,omitempty"`

	// Autostart determines if the VM should start automatically on host boot.
	// Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Autostart *bool `json:"autostart,omitempty" yaml:"autostart,omitempty"`

	// Firmware pins the UEFI firmware and its varstore template instead of
	// letting libvirt auto-select them (firmware='efi'), and can turn on
	// Secure Boot. Without it the VM gets libvirt's auto-selected UEFI
	// firmware, as before.
	// +optional
	Firmware *FirmwareSpec `json:"firmware,omitempty" yaml:"firmware,omitempty"`

	// Serial is the VM's SMBIOS system serial number (type 1), what the guest
	// reads as /sys/class/dmi/id/product_serial. A guest that binds something
	// to its machine (an installer that checks which machine it runs on)
	// needs one that is set and stable; QEMU sets none by default. 1 to 64
	// characters from A-Z a-z 0-9 . _ -. When SMBIOS is set and Serial is
	// not, the serial is "foundry-" followed by metadata.name, so it is known
	// before the VM exists and stays the same when the VM is recreated. With
	// neither set the VM gets no SMBIOS system information, as before.
	// +optional
	Serial string `json:"serial,omitempty" yaml:"serial,omitempty"`

	// SMBIOS sets the rest of the VM's SMBIOS system information (type 1)
	// next to Serial: manufacturer, product, version, SKU, family and UUID,
	// e.g. to make a test guest report the same machine identity as the
	// hardware it stands in for. Unset fields keep QEMU's values. An empty
	// SMBIOS ("smbios: {}") gives the VM just the default serial.
	// +optional
	SMBIOS *SMBIOSSpec `json:"smbios,omitempty" yaml:"smbios,omitempty"`

	// FwCfg lists QEMU fw_cfg entries given to the guest on every boot. The
	// guest reads each from /sys/firmware/qemu_fw_cfg/by_name/<name>/raw.
	// +optional
	FwCfg []FwCfgEntrySpec `json:"fwCfg,omitempty" yaml:"fwCfg,omitempty"`

	// InputDisks are small read-only disks, each an ISO-9660 image with a
	// volume label and a few files (e.g. an installer's config), that foundry
	// builds, uploads into the VM's storage pool as a volume and attaches as
	// a CD-ROM. The guest finds one by its label
	// (/dev/disk/by-label/<label>). Deleted with the VM. Without it the VM
	// gets no input disks, as before.
	// +optional
	InputDisks []InputDiskSpec `json:"inputDisks,omitempty" yaml:"inputDisks,omitempty"`
}

// InputDiskSpec defines a small read-only ISO-9660 disk with files for the
// guest.
//
// +k8s:deepcopy-gen=true
type InputDiskSpec struct {
	// Label is the ISO-9660 volume label the guest finds the disk by
	// (/dev/disk/by-label/<label>): 1 to 32 characters from A-Z 0-9 _
	// (ISO-9660 d-characters). Unique among the VM's input disks; also names
	// the volume (<vm>_input-<label in lower case>.iso). CIDATA is only
	// allowed without spec.cloudInit (it is the label of foundry's own
	// cloud-init seed).
	Label string `json:"label" yaml:"label"`

	// Files are the disk's files, all in its root directory. At least one.
	Files []InputFileSpec `json:"files" yaml:"files"`
}

// InputFileSpec defines one file of an input disk.
// Exactly one of File or String must be set.
//
// +k8s:deepcopy-gen=true
type InputFileSpec struct {
	// Name is the file name in the disk's root directory. ISO-9660 without
	// extensions keeps only short lower-case names intact on a Linux guest,
	// so it is a base of a-z 0-9 _ - and an optional extension of a dot and
	// 1 to 8 of the same characters, 28 characters at most (e.g. "node.yaml").
	Name string `json:"name" yaml:"name"`

	// File is a regular file, on the machine foundry runs on, to copy in.
	// foundry reads it itself and uploads the disk image through libvirt.
	// A relative path is taken from the directory of the VM's YAML file.
	// +optional
	File string `json:"file,omitempty" yaml:"file,omitempty"`

	// String is the file's content, given inline.
	// +optional
	String string `json:"string,omitempty" yaml:"string,omitempty"`
}

// SMBIOSSpec defines SMBIOS system information (type 1) entries, rendered as
// libvirt <sysinfo type='smbios'><system> entries
// (https://libvirt.org/formatdomain.html#smbios-system-information).
// Strings are 1 to 64 characters from A-Z a-z 0-9, space and . _ - + ( ) / :,
// with no leading or trailing space.
//
// +k8s:deepcopy-gen=true
type SMBIOSSpec struct {
	// Manufacturer is the system manufacturer (/sys/class/dmi/id/sys_vendor).
	// +optional
	Manufacturer string `json:"manufacturer,omitempty" yaml:"manufacturer,omitempty"`

	// Product is the product name (/sys/class/dmi/id/product_name).
	// +optional
	Product string `json:"product,omitempty" yaml:"product,omitempty"`

	// Version is the product version (/sys/class/dmi/id/product_version).
	// +optional
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// SKU is the SKU number (/sys/class/dmi/id/product_sku).
	// +optional
	SKU string `json:"sku,omitempty" yaml:"sku,omitempty"`

	// Family is the product family (/sys/class/dmi/id/product_family).
	// +optional
	Family string `json:"family,omitempty" yaml:"family,omitempty"`

	// UUID is the system UUID (/sys/class/dmi/id/product_uuid), in the
	// 8-4-4-4-12 hexadecimal form. libvirt requires the SMBIOS UUID to match
	// the domain's own UUID, so this also becomes the domain UUID: a VM that
	// is destroyed and created again keeps the same identity. Default: the
	// UUID libvirt gives the domain when it is defined.
	// +optional
	UUID string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
}

// FwCfgEntrySpec defines a named QEMU fw_cfg entry.
// Exactly one of File or String must be set.
//
// +k8s:deepcopy-gen=true
type FwCfgEntrySpec struct {
	// Name is the fw_cfg item name; it must start with "opt/"
	// (e.g. "opt/org.tianocore/EnableLegacyLoader"). libvirt reserves
	// "opt/ovmf/" and "opt/org.qemu/".
	Name string `json:"name" yaml:"name"`

	// File is a file on the hypervisor whose content becomes the entry.
	// +optional
	File string `json:"file,omitempty" yaml:"file,omitempty"`

	// String is the entry's content, given inline.
	// +optional
	String string `json:"string,omitempty" yaml:"string,omitempty"`
}

// FirmwareSpec defines an explicit UEFI firmware (pflash) configuration.
//
// +k8s:deepcopy-gen=true
type FirmwareSpec struct {
	// Loader is the path of the firmware code image on the hypervisor, for
	// the guest's architecture (e.g. "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd"
	// for x86_64, "/usr/share/edk2/aarch64/QEMU_EFI-silent-pflash.raw" for
	// aarch64).
	Loader string `json:"loader" yaml:"loader"`

	// SecureBoot marks the loader as secure (only the guest's SMM code may
	// write the varstore) and turns SMM on, as Secure Boot requires.
	// x86_64 only. Defaults to false.
	// +optional
	SecureBoot bool `json:"secureBoot,omitempty" yaml:"secureBoot,omitempty"`

	// NVRAMTemplate is the varstore file on the hypervisor copied to the
	// VM's own varstore on first start (e.g. one with a custom Secure Boot
	// db enrolled). Defaults to the template libvirt pairs with the loader.
	// Mutually exclusive with Keys.
	// +optional
	NVRAMTemplate string `json:"nvramTemplate,omitempty" yaml:"nvramTemplate,omitempty"`

	// Keys names a Secure Boot key profile: a varstore with a set of PK, KEK,
	// db and dbx entries enrolled, imported once with 'foundry varstore
	// import' and kept as a volume of the foundry-images pool. foundry looks
	// the volume up for the guest architecture (spec.arch) and uses it as
	// the VM's varstore template. Lowercase letters, digits and dashes, at
	// most 63 characters. Mutually exclusive with NVRAMTemplate.
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Keys string `json:"keys,omitempty" yaml:"keys,omitempty"`
}

// BootDiskSpec defines the boot disk configuration.
//
// +k8s:deepcopy-gen=true
type BootDiskSpec struct {
	// SizeGB is the size of the boot disk in gigabytes.
	// +kubebuilder:validation:Minimum=1
	SizeGB int `json:"sizeGB" yaml:"sizeGB"`

	// Image is the base image to use for the boot disk.
	// Can be a volume name (e.g., "fedora-43.qcow2"),
	// a pool:volume reference (e.g., "foundry-images:fedora-43.qcow2"),
	// or a file path (e.g., "/var/lib/libvirt/images/fedora-43.qcow2").
	// Mutually exclusive with Empty.
	// +optional
	Image string `json:"image,omitempty" yaml:"image,omitempty"`

	// ImagePool is the storage pool containing the base image.
	// Defaults to "foundry-images" if not specified.
	// Only used when Image is a volume name without pool prefix.
	// +optional
	// +kubebuilder:default=foundry-images
	ImagePool string `json:"imagePool,omitempty" yaml:"imagePool,omitempty"`

	// Format is the disk format to use.
	// Valid values: "qcow2" (default), "raw".
	// +optional
	// +kubebuilder:validation:Enum=qcow2;raw
	// +kubebuilder:default=qcow2
	Format string `json:"format,omitempty" yaml:"format,omitempty"`

	// Empty creates an empty boot disk instead of using a base image.
	// Mutually exclusive with Image.
	// +optional
	Empty bool `json:"empty,omitempty" yaml:"empty,omitempty"`

	// Serial is the disk serial number the guest sees (libvirt <serial>),
	// e.g. in /dev/disk/by-id. 1-20 characters from A-Z a-z 0-9 - _ .;
	// unique among the VM's disks.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9._-]{1,20}$`
	Serial string `json:"serial,omitempty" yaml:"serial,omitempty"`
}

// DataDiskSpec defines an additional data disk configuration.
//
// +k8s:deepcopy-gen=true
type DataDiskSpec struct {
	// Device is the device name for the disk (e.g., "vdb", "vdc").
	// Must be unique within the VM.
	Device string `json:"device" yaml:"device"`

	// SizeGB is the size of the data disk in gigabytes.
	// +kubebuilder:validation:Minimum=1
	SizeGB int `json:"sizeGB" yaml:"sizeGB"`

	// Serial is the disk serial number the guest sees (libvirt <serial>),
	// so the guest can find the disk by a stable name in /dev/disk/by-id
	// rather than by probe order. 1-20 characters from A-Z a-z 0-9 - _ .
	// (a virtio-blk serial holds at most 20 bytes; QEMU cuts longer ones);
	// unique among the VM's disks.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9._-]{1,20}$`
	Serial string `json:"serial,omitempty" yaml:"serial,omitempty"`
}

// NetworkInterfaceSpec defines a network interface configuration.
//
// +k8s:deepcopy-gen=true
type NetworkInterfaceSpec struct {
	// IP is the IP address with CIDR notation (e.g., "10.250.250.10/24").
	// Used to derive MAC address and interface name deterministically.
	IP string `json:"ip" yaml:"ip"`

	// Gateway is the default gateway IP address.
	Gateway string `json:"gateway" yaml:"gateway"`

	// Bridge is the bridge name to attach the interface to.
	Bridge string `json:"bridge" yaml:"bridge"`

	// Vlan is the 802.1Q VLAN ID to tag this interface with on the bridge.
	// When set, libvirt applies the tag to the bridge port, so the guest sees
	// untagged traffic and the generated network-config is unaffected.
	// The bridge must be VLAN-aware and trunk the given ID.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	// +optional
	Vlan *uint `json:"vlan,omitempty" yaml:"vlan,omitempty"`

	// DNSServers is the list of DNS server IP addresses.
	// +optional
	DNSServers []string `json:"dnsServers,omitempty" yaml:"dnsServers,omitempty"`

	// DefaultRoute determines if this interface should have the default route.
	// Only one interface should have this set to true.
	// Defaults to false.
	// +optional
	DefaultRoute bool `json:"defaultRoute,omitempty" yaml:"defaultRoute,omitempty"`

	// HostDevice fixes the name of the interface's host-side device (the tap
	// device libvirt creates and plugs into the bridge; libvirt's
	// <interface><target dev=.../>, https://libvirt.org/formatdomain.html#overriding-the-target-element),
	// for a caller that finds the VM's port on the host by name. Without it
	// the name is derived from the IP ("vm" and its four octets in hex, e.g.
	// vm0a371616). A Linux interface name: 1 to 15 characters, not "." or
	// "..", without "/", ":" or white space, and not starting with "vnet",
	// "vif", "macvtap" or "macvlan": libvirt reserves those prefixes for the
	// names it generates and drops such a name from the persistent
	// definition, so the device would get a generated name that changes
	// from one start to the next. Unique among the VM's interfaces; that no
	// other VM uses it is the caller's concern (the VM then fails to start).
	// +optional
	// +kubebuilder:validation:MaxLength=15
	HostDevice string `json:"hostDevice,omitempty" yaml:"hostDevice,omitempty"`

	// PXEBoot enables network boot (PXE) on this interface.
	// When true, this interface will be configured as the primary boot device
	// with boot order 1, and the boot disk will be set to boot order 2.
	// Only one interface should have this set to true.
	// Defaults to false.
	// +optional
	PXEBoot bool `json:"pxeBoot,omitempty" yaml:"pxeBoot,omitempty"`
}

// CloudInitSpec defines cloud-init configuration.
//
// +k8s:deepcopy-gen=true
type CloudInitSpec struct {
	// RawUserData allows providing complete custom user-data content.
	// When set, this overrides the generated user-data from other fields.
	// Must be valid cloud-init format (e.g., start with #cloud-config).
	// Meta-data and network-config are still generated automatically.
	// +optional
	RawUserData string `json:"rawUserData,omitempty" yaml:"rawUserData,omitempty"`

	// FQDN is the fully qualified domain name for the VM.
	// The hostname is derived from this.
	// Ignored if RawUserData is set.
	// +optional
	FQDN string `json:"fqdn,omitempty" yaml:"fqdn,omitempty"`

	// SSHAuthorizedKeys is a list of SSH public keys to inject.
	// Ignored if RawUserData is set.
	// +optional
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty" yaml:"sshAuthorizedKeys,omitempty"`

	// PasswordHash is the hashed password for the root user.
	// Generate with: mkpasswd --method=SHA-512
	// Ignored if RawUserData is set.
	// +optional
	PasswordHash string `json:"passwordHash,omitempty" yaml:"passwordHash,omitempty"`

	// SSHPasswordAuth enables SSH password authentication.
	// Defaults to false (key-only authentication).
	// Ignored if RawUserData is set.
	// +optional
	SSHPasswordAuth bool `json:"sshPasswordAuth,omitempty" yaml:"sshPasswordAuth,omitempty"`
}

// VirtualMachineStatus defines the observed state of a VirtualMachine.
//
// +k8s:deepcopy-gen=true
type VirtualMachineStatus struct {
	// Phase represents the current lifecycle phase of the VM.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Creating;Running;Stopping;Stopped;Failed
	Phase VMPhase `json:"phase,omitempty" yaml:"phase,omitempty"`

	// Conditions represent the latest available observations of the VM's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	// Addresses are the network addresses assigned to the VM.
	// +optional
	Addresses []VMAddress `json:"addresses,omitempty" yaml:"addresses,omitempty"`

	// DomainUUID is the libvirt domain UUID.
	// Populated after VM creation.
	// +optional
	DomainUUID string `json:"domainUUID,omitempty" yaml:"domainUUID,omitempty"`

	// MACAddresses are the MAC addresses assigned to each network interface.
	// Calculated deterministically from IP addresses.
	// +optional
	MACAddresses []string `json:"macAddresses,omitempty" yaml:"macAddresses,omitempty"`

	// InterfaceNames are the tap interface names for each network interface.
	// Calculated deterministically from IP addresses.
	// +optional
	InterfaceNames []string `json:"interfaceNames,omitempty" yaml:"interfaceNames,omitempty"`

	// ObservedGeneration reflects the generation most recently observed by Foundry.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

// VMPhase represents the lifecycle phase of a VirtualMachine.
type VMPhase string

const (
	// VMPhasePending means the VM has been accepted but not yet created.
	VMPhasePending VMPhase = "Pending"

	// VMPhaseCreating means the VM is currently being created.
	VMPhaseCreating VMPhase = "Creating"

	// VMPhaseRunning means the VM has been created and is running.
	VMPhaseRunning VMPhase = "Running"

	// VMPhaseStopping means the VM is being gracefully shut down.
	VMPhaseStopping VMPhase = "Stopping"

	// VMPhaseStopped means the VM has been stopped.
	VMPhaseStopped VMPhase = "Stopped"

	// VMPhaseFailed means the VM is in a failed state and needs intervention.
	VMPhaseFailed VMPhase = "Failed"
)

// VMAddress represents a network address assigned to the VM.
//
// +k8s:deepcopy-gen=true
type VMAddress struct {
	// Type is the type of address.
	// Valid values: "InternalIP", "ExternalIP", "Hostname".
	Type string `json:"type" yaml:"type"`

	// Address is the actual address value (IP or hostname).
	Address string `json:"address" yaml:"address"`
}

// Standard condition types for VirtualMachine resources.
const (
	// ConditionReady indicates that the VM is fully operational and ready.
	ConditionReady = "Ready"

	// ConditionStorageProvisioned indicates that all storage volumes have been created.
	ConditionStorageProvisioned = "StorageProvisioned"

	// ConditionNetworkConfigured indicates that network interfaces are configured.
	ConditionNetworkConfigured = "NetworkConfigured"

	// ConditionCloudInitReady indicates that the cloud-init ISO has been created.
	ConditionCloudInitReady = "CloudInitReady"
)

// DeepCopy creates a deep copy of VirtualMachine.
func (in *VirtualMachine) DeepCopy() *VirtualMachine {
	if in == nil {
		return nil
	}
	out := new(VirtualMachine)
	out.TypeMeta = *in.TypeMeta.DeepCopy()
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = *in.Spec.DeepCopy()
	out.Status = *in.Status.DeepCopy()
	return out
}

// DeepCopy creates a deep copy of VirtualMachineSpec.
func (in *VirtualMachineSpec) DeepCopy() *VirtualMachineSpec {
	if in == nil {
		return nil
	}
	out := new(VirtualMachineSpec)
	*out = *in

	// Deep copy BootDisk
	out.BootDisk = *in.BootDisk.DeepCopy()

	// Deep copy DataDisks slice
	if in.DataDisks != nil {
		out.DataDisks = make([]DataDiskSpec, len(in.DataDisks))
		for i := range in.DataDisks {
			out.DataDisks[i] = *in.DataDisks[i].DeepCopy()
		}
	}

	// Deep copy NetworkInterfaces slice
	if in.NetworkInterfaces != nil {
		out.NetworkInterfaces = make([]NetworkInterfaceSpec, len(in.NetworkInterfaces))
		for i := range in.NetworkInterfaces {
			out.NetworkInterfaces[i] = *in.NetworkInterfaces[i].DeepCopy()
		}
	}

	// Deep copy CloudInit
	if in.CloudInit != nil {
		out.CloudInit = in.CloudInit.DeepCopy()
	}

	// Deep copy Autostart pointer
	if in.Autostart != nil {
		autostart := *in.Autostart
		out.Autostart = &autostart
	}

	// Deep copy InputDisks slice
	if in.InputDisks != nil {
		out.InputDisks = make([]InputDiskSpec, len(in.InputDisks))
		for i := range in.InputDisks {
			out.InputDisks[i] = *in.InputDisks[i].DeepCopy()
		}
	}

	return out
}

// DeepCopy creates a deep copy of InputDiskSpec.
func (in *InputDiskSpec) DeepCopy() *InputDiskSpec {
	if in == nil {
		return nil
	}
	out := new(InputDiskSpec)
	*out = *in
	if in.Files != nil {
		out.Files = make([]InputFileSpec, len(in.Files))
		copy(out.Files, in.Files)
	}
	return out
}

// DeepCopy creates a deep copy of BootDiskSpec.
func (in *BootDiskSpec) DeepCopy() *BootDiskSpec {
	if in == nil {
		return nil
	}
	out := new(BootDiskSpec)
	*out = *in
	return out
}

// DeepCopy creates a deep copy of DataDiskSpec.
func (in *DataDiskSpec) DeepCopy() *DataDiskSpec {
	if in == nil {
		return nil
	}
	out := new(DataDiskSpec)
	*out = *in
	return out
}

// DeepCopy creates a deep copy of NetworkInterfaceSpec.
func (in *NetworkInterfaceSpec) DeepCopy() *NetworkInterfaceSpec {
	if in == nil {
		return nil
	}
	out := new(NetworkInterfaceSpec)
	*out = *in

	// Deep copy DNSServers slice
	if in.DNSServers != nil {
		out.DNSServers = make([]string, len(in.DNSServers))
		copy(out.DNSServers, in.DNSServers)
	}

	return out
}

// DeepCopy creates a deep copy of CloudInitSpec.
func (in *CloudInitSpec) DeepCopy() *CloudInitSpec {
	if in == nil {
		return nil
	}
	out := new(CloudInitSpec)
	*out = *in

	// Deep copy SSHAuthorizedKeys slice
	if in.SSHAuthorizedKeys != nil {
		out.SSHAuthorizedKeys = make([]string, len(in.SSHAuthorizedKeys))
		copy(out.SSHAuthorizedKeys, in.SSHAuthorizedKeys)
	}

	return out
}

// DeepCopy creates a deep copy of VirtualMachineStatus.
func (in *VirtualMachineStatus) DeepCopy() *VirtualMachineStatus {
	if in == nil {
		return nil
	}
	out := new(VirtualMachineStatus)
	*out = *in

	// Deep copy Conditions slice
	if in.Conditions != nil {
		out.Conditions = make([]Condition, len(in.Conditions))
		for i := range in.Conditions {
			out.Conditions[i] = *in.Conditions[i].DeepCopy()
		}
	}

	// Deep copy Addresses slice
	if in.Addresses != nil {
		out.Addresses = make([]VMAddress, len(in.Addresses))
		for i := range in.Addresses {
			out.Addresses[i] = *in.Addresses[i].DeepCopy()
		}
	}

	// Deep copy MACAddresses slice
	if in.MACAddresses != nil {
		out.MACAddresses = make([]string, len(in.MACAddresses))
		copy(out.MACAddresses, in.MACAddresses)
	}

	// Deep copy InterfaceNames slice
	if in.InterfaceNames != nil {
		out.InterfaceNames = make([]string, len(in.InterfaceNames))
		copy(out.InterfaceNames, in.InterfaceNames)
	}

	return out
}

// DeepCopy creates a deep copy of VMAddress.
func (in *VMAddress) DeepCopy() *VMAddress {
	if in == nil {
		return nil
	}
	out := new(VMAddress)
	*out = *in
	return out
}
