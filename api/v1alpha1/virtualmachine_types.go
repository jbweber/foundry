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

	// CPUMode defines the CPU model exposure mode.
	// Valid values: "host-model" (default), "host-passthrough".
	// +optional
	// +kubebuilder:validation:Enum=host-model;host-passthrough
	// +kubebuilder:default=host-model
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

	// DNSServers is the list of DNS server IP addresses.
	// +optional
	DNSServers []string `json:"dnsServers,omitempty" yaml:"dnsServers,omitempty"`

	// DefaultRoute determines if this interface should have the default route.
	// Only one interface should have this set to true.
	// Defaults to false.
	// +optional
	DefaultRoute bool `json:"defaultRoute,omitempty" yaml:"defaultRoute,omitempty"`
}

// CloudInitSpec defines cloud-init configuration.
//
// +k8s:deepcopy-gen=true
type CloudInitSpec struct {
	// FQDN is the fully qualified domain name for the VM.
	// The hostname is derived from this.
	// +optional
	FQDN string `json:"fqdn,omitempty" yaml:"fqdn,omitempty"`

	// SSHAuthorizedKeys is a list of SSH public keys to inject.
	// +optional
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty" yaml:"sshAuthorizedKeys,omitempty"`

	// PasswordHash is the hashed password for the root user.
	// Generate with: mkpasswd --method=SHA-512
	// +optional
	PasswordHash string `json:"passwordHash,omitempty" yaml:"passwordHash,omitempty"`

	// SSHPasswordAuth enables SSH password authentication.
	// Defaults to false (key-only authentication).
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
