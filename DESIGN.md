# Foundry Design Document

## Overview

Foundry is a Go-based CLI tool for managing libvirt VMs, replacing the Ansible-based workflow in the homestead project. It provides simple commands to create, destroy, and list VMs using YAML configuration files.

## Architecture

### Pure Go Stack (No CGo, Minimal External Dependencies)

**Core Libraries:**
- `github.com/digitalocean/go-libvirt` - Pure Go libvirt client (no CGo)
- `github.com/libvirt/libvirt-go-xml` - Libvirt XML domain generation
- `github.com/kdomanski/iso9660` - Pure Go ISO creation for cloud-init
- `gopkg.in/yaml.v3` - YAML parsing

**What libvirt handles directly:**
- Create/delete/start/stop VMs
- Create storage pools & volumes (qcow2 with backing files)
- Network interface management
- Volume upload/download

**External tools (only if needed):**
- `qemu-img` - Only for advanced operations like resize/convert (optional)

### Project Structure

```
foundry/
├── cmd/foundry/
│   ├── main.go              # CLI entrypoint with subcommands
│   ├── create.go            # VM create command
│   ├── destroy.go           # VM destroy command
│   ├── list.go              # VM list command
│   ├── get.go               # VM get command (single VM details)
│   ├── pool.go              # Pool management commands
│   ├── image.go             # Image management commands
│   └── storage.go           # Storage status command
├── api/
│   └── v1alpha1/
│       ├── types.go         # VirtualMachine K8s-style API types
│       ├── types_test.go    # API type tests
│       └── helpers.go       # Helper methods (MAC calculation, volume naming, etc.)
├── internal/
│   ├── loader/
│   │   └── loader.go        # YAML loader for v1alpha1 format
│   ├── metadata/
│   │   └── storage.go       # Libvirt XML metadata storage + consumer interface
│   ├── status/
│   │   ├── conditions.go    # Condition management
│   │   └── phase.go         # Phase management (Pending, Running, etc.)
│   ├── output/
│   │   ├── table.go         # Table output formatter
│   │   ├── yaml.go          # YAML output formatter
│   │   └── json.go          # JSON output formatter
│   ├── storage/
│   │   ├── types.go         # Storage types (PoolType, VolumeSpec, etc.)
│   │   ├── manager.go       # Storage manager + consumer interface
│   │   ├── pool.go          # Pool operations (create, list, delete)
│   │   ├── volume.go        # Volume operations (create, delete, upload)
│   │   └── image.go         # Base image management (import, pull, list)
│   ├── cloudinit/
│   │   ├── generator.go     # Generate user-data, meta-data, network-config
│   │   └── iso.go           # Create ISO using iso9660
│   ├── libvirtxml/
│   │   └── domain.go        # Domain XML generation from VirtualMachine spec
│   ├── naming/
│   │   └── naming.go        # Resource naming (MAC, interface, volume names)
│   └── vm/
│       ├── create.go        # VM creation orchestration
│       ├── destroy.go       # VM destruction logic
│       ├── list.go          # VM listing with status population
│       ├── get.go           # Get single VM with status
│       └── interfaces.go    # Consumer-side LibvirtClient interface
├── examples/
│   ├── simple-vm.yaml       # Basic VM config example
│   ├── multi-disk-vm.yaml   # VM with data disks
│   ├── custom-pool-vm.yaml  # VM using custom storage pool
│   └── config.yaml          # Foundry configuration example
├── go.mod
├── go.sum
├── DESIGN.md                # This file
├── CLOUDINIT.md             # Cloud-init implementation details
├── PLAN_FOUNDRY.md          # Implementation plan and progress
└── README.md                # User documentation
```

## Configuration Format

### VM Configuration YAML

Foundry uses a Kubernetes-style API format with `apiVersion`, `kind`, `metadata`, `spec`, and `status` sections:

```yaml
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: my-vm                 # VM name (libvirt domain name, normalized to lowercase)
  labels:                     # Optional: key-value labels for organization
    environment: production
    role: webserver
  annotations:                # Optional: arbitrary metadata
    description: "Production web server"

spec:
  # Resource allocation
  vcpus: 4                    # Number of virtual CPUs
  memoryGiB: 8                # Memory in GiB

  # Boot disk configuration
  bootDisk:
    sizeGB: 50                # Disk size in GB
    image: fedora-43.qcow2    # Image reference (see formats below)
    imagePool: foundry-images # Optional: pool containing base image (default: foundry-images)
    # OR for empty boot disk:
    # empty: true             # Create empty disk instead of snapshot

  # Optional: Additional data disks
  dataDisks:
    - device: vdb             # Device name (vdb, vdc, etc.)
      sizeGB: 100
    - device: vdc
      sizeGB: 200

  # Network configuration
  networkInterfaces:
    - ip: 10.20.30.40/24      # IP with CIDR
      gateway: 10.20.30.1     # Gateway IP
      dnsServers:             # DNS servers
        - 8.8.8.8
        - 1.1.1.1
      bridge: br0             # Bridge name to attach to
      defaultRoute: true      # Set default route (optional, default: true for first interface)

    # Optional: Additional interfaces
    - ip: 192.168.1.50/24
      gateway: 192.168.1.1
      dnsServers:
        - 192.168.1.1
      bridge: br1
      defaultRoute: false

  # Optional: Cloud-init configuration
  cloudInit:
    fqdn: my-vm.example.com   # FQDN (hostname derived from this, normalized to lowercase)
    sshAuthorizedKeys:        # SSH public keys to inject
      - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo..."
      - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC..."
    passwordHash: "$6$..."    # Optional: Root password hash (mkpasswd --method=SHA-512)
    sshPwauth: false          # Optional: Enable SSH password auth (default: false)

  # Optional: Advanced settings
  cpuMode: host-model         # CPU mode: host-model (default), host-passthrough
  autostart: true             # Auto-start VM on host boot (default: true)
  storagePool: foundry-vms    # Storage pool to use (default: foundry-vms)

# Status (populated automatically by foundry)
status:
  phase: Running              # Pending, Creating, Running, Stopped, Failed, Destroying
  conditions:                 # Array of condition objects
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-11-03T10:30:00Z"
      reason: VMRunning
      message: "VM is running successfully"
  observedGeneration: 1       # Last generation observed by controller
```

**Image Reference Formats:**
- Volume name: `fedora-43.qcow2` (uses `imagePool` or defaults to `foundry-images`)
- Full reference: `foundry-images:fedora-43.qcow2` (explicit pool:volume format)
- File path: `/path/to/image.qcow2` (direct filesystem path, not recommended)

### Configuration Validation Rules

**Required:**
- `metadata.name`
- `spec.vcpus`, `spec.memoryGiB`
- `spec.bootDisk.sizeGB`
- `spec.bootDisk.image` OR `spec.bootDisk.empty: true`
- At least one `spec.networkInterfaces` entry with `ip`, `gateway`, `bridge`

**Normalization (automatic):**
- `metadata.name` → lowercase
- `spec.cloudInit.fqdn` → lowercase (hostname derived from this)

**Validation checks:**
- `metadata.name` format: `^[a-z0-9][a-z0-9_-]*[a-z0-9]$` (after normalization)
  - Must start and end with alphanumeric
  - Can contain alphanumeric, hyphens, underscores
- `spec.cloudInit.fqdn` format: valid FQDN (hostname + domain with dots)
- VCPUs > 0, memoryGiB > 0, disk sizes > 0
- IP addresses valid with CIDR notation
- No duplicate device names in data disks
- No duplicate IP addresses in network interfaces
- SSH keys have valid format (ssh-rsa, ssh-ed25519, etc.)
- Password hash starts with `$` (crypt format)

**Runtime validation (during VM creation):**
- VM name doesn't conflict with existing domain
- Boot disk image exists (unless empty: true)
- Bridge exists on hypervisor (future: fuzzy match)

## Core Workflows

### VM Creation Workflow

```
1. Parse YAML config
2. Validate configuration
   - Check required fields
   - Validate IPs, CIDRs
   - Check base image exists
   - Check VM name doesn't exist
3. Calculate derived values
   - MAC address from IP (be:ef:XX:XX:XX:XX)
   - VM directory path
4. Create storage via libvirt
   - Create VM storage pool (if needed)
   - Create boot disk volume (qcow2 with backing file)
   - Create data disk volumes
5. Generate cloud-init ISO (if enabled)
   - Generate user-data YAML
   - Generate meta-data YAML
   - Generate network-config YAML
   - Create ISO using iso9660 library
   - Upload ISO to libvirt storage
6. Generate libvirt domain XML
   - CPU, memory, boot order
   - Disk devices (virtio)
   - Network interfaces (bridge, virtio)
   - Cloud-init CDROM
   - Serial console
7. Define domain in libvirt
8. Set autostart flag
9. Start domain
```

### VM Destruction Workflow

```
1. Check VM exists
2. Get domain info
3. If running:
   - Attempt graceful shutdown
   - Wait 5 seconds
   - If still running, force destroy
4. Undefine domain with NVRAM cleanup
5. Delete storage volumes
   - Boot disk
   - Data disks
   - Cloud-init ISO
6. Remove VM directory
```

### VM Listing Workflow

```
1. Connect to libvirt
2. List all domains
3. For each domain:
   - Get name, state, UUID
   - Get CPU count
   - Get memory
   - Get IP addresses (if available)
4. Display formatted table
```

## Implementation Details

### MAC Address Calculation

Algorithm (matching Ansible implementation):
```
IP: 10.55.22.22
Convert to hex: 0a 37 16 16
MAC: be:ef:0a:37:16:16
```

This ensures:
- Deterministic MAC from IP
- Unique MACs per VM
- Compatible with existing homestead VMs

### Network Interface Naming

Foundry automatically generates tap interface names for all network interfaces based on their IP addresses.

**Algorithm** (matching Ansible implementation):
```
IP: 10.55.22.22
Convert to hex: 0a 37 16 16
Interface name: vm0a371616
```

**Format**: `vm{hex octets}`
- Prefix: `vm` (2 chars)
- IP octets in hexadecimal (8 chars)
- Total length: 10 characters (well within Linux kernel's 15-char limit)

**Examples**:
```
IP: 10.55.22.22      → Interface: vm0a371616
IP: 10.250.250.10    → Interface: vm0afafa0a
IP: 192.168.1.100    → Interface: vmc0a80164
```

**Benefits**:
- **Deterministic**: Same IP always produces same interface name
- **Identifiable**: Can decode interface name back to IP address
- **Battle-tested**: Matches existing Ansible homestead implementation
- **Kernel-compliant**: 10 chars fits easily within 15-char Linux kernel limit

**Usage in Libvirt**:
- All interfaces (bridge and ethernet modes) include `<target dev="vm..."/>`
- For bridge mode: Provides predictable interface name for monitoring/debugging
- For ethernet mode: Required for manual bridge attachment and BGP routing

**Limitations**:
⚠️ **IP Uniqueness Required**: This approach assumes IP addresses are unique across all VMs. If two VMs use the same IP address (even on different networks/bridges), they will have interface name collisions.

This is acceptable for the current use case where IPs are expected to be unique.

**Future Enhancement**:
When a database/state store is available, consider migrating to a better naming scheme such as:
- VM name-based hashing (e.g., `vm-webserver-4a2b`)
- Sequential allocation with persistent mapping
- Database-backed lookups for efficient reverse mapping

A database would enable collision-free name generation and efficient interface-to-VM lookups.

### Storage Management

**Architecture**: Unified libvirt storage pool approach with separate pools for images and VMs.

**Two Default Pools** (auto-created on first use):
- `foundry-images` - Base OS images at `/var/lib/libvirt/images/foundry/images/`
  - Read-mostly, shared across VMs
  - Small total size (few GB)
- `foundry-vms` - VM disks at `/var/lib/libvirt/images/foundry/vms/`
  - Read-write active disks
  - Most I/O happens here
  - Default pool for VM creation

**Custom Pools** (user-managed):
- Users can add pools for specific storage needs (SSD, bulk, network storage)
- Support for different backends: dir, LVM, ZFS, NFS, Ceph
- CLI commands for pool management

**Volume Naming Convention** (flat namespace within pools):
- Boot disk: `{vm-name}_boot`
- Data disks: `{vm-name}_data-{device}` (e.g., `web-server_data-vdb`)
- Cloud-init ISO: `{vm-name}_cloudinit`
- Base images: `{os-name}-{version}` (e.g., `fedora-43`, `ubuntu-24.04`)

**Storage structure:**
```
/var/lib/libvirt/images/foundry/
├── images/                    # foundry-images pool
│   ├── fedora-43              # Base image volume
│   ├── ubuntu-24.04           # Base image volume
│   └── debian-12              # Base image volume
└── vms/                       # foundry-vms pool
    ├── my-vm_boot             # Boot disk volume (qcow2 with backing: fedora-43)
    ├── my-vm_data-vdb         # Data disk volume
    ├── my-vm_data-vdc         # Data disk volume
    ├── my-vm_cloudinit        # Cloud-init ISO volume
    ├── prod-web01_boot        # Another VM's boot disk
    └── prod-web01_cloudinit   # Another VM's cloud-init
```

**Benefits:**
- ✅ Libvirt-native (no shell commands, proper error handling)
- ✅ Multiple storage backends (dir, LVM, ZFS, NFS, Ceph)
- ✅ Clean separation (images vs VM disks in different pools)
- ✅ Auto-management (permissions handled by libvirt)
- ✅ Flexible storage (users can choose fast/slow/network per VM)
- ✅ Future-proof (snapshots, cloning, migration supported)

**Configuration Layers** (priority order):
1. **CLI flags** (highest priority): `foundry create --pool foundry-ssd`
2. **Environment variables**: `FOUNDRY_VM_POOL=my-pool`
3. **Config file** (`~/.config/foundry/config.yaml` or `/etc/foundry/config.yaml`)
4. **Hard-coded defaults** (lowest priority): `foundry-vms`, `foundry-images`

**Image Format Validation** (added v0.2.0):

Foundry validates disk images on import to ensure they are bootable OS images:

- **Pure Go detection** - No external commands, reads magic bytes directly
- **QCOW2 format** - Checks for `QFI\xfb` (0x51 0x46 0x49 0xfb) at offset 0
  - Reference: [QEMU QCOW2 specification](https://www.qemu.org/docs/master/interop/qcow2.html)
- **RAW format** - Checks for MBR boot signature `0x55aa` at offset 510
  - Works for both MBR and GPT disks (GPT has protective MBR)
  - Reference: [UEFI GPT specification](https://uefi.org/specs/UEFI/2.10/05_GUID_Partition_Table_Format.html)
- **Required extension** - Image names must have `.qcow2` or `.raw` extension matching actual format
- **Format mismatch detection** - Rejects files where extension doesn't match detected format
- **Non-bootable rejection** - RAW images without boot sector signature are rejected

This prevents common issues like:
- Importing non-bootable files that cause VM boot failures
- Misnamed files (e.g., QCOW2 with `.raw` extension)
- Arbitrary data files being imported as images

**Future enhancements**:
- Support for additional formats (VMDK, VDI, VHD) by adding their magic bytes
- Optional format conversion on import (`qemu-img convert`)
- Deep validation of image integrity (beyond magic bytes)
- RAW-to-QCOW2 conversion workflow for production use (with sparsification)

### Cloud-init Generation

**Three files in ISO:**

1. **user-data** (cloud-config YAML):
```yaml
#cloud-config
hostname: my-vm
fqdn: my-vm.example.com
users:
  - name: root
    ssh_authorized_keys:
      - ssh-ed25519 AAAA...
chpasswd:
  list: |
    root:$6$...
  expire: false
ssh_pwauth: false
```

2. **meta-data** (instance metadata):
```yaml
instance-id: my-vm-1730534400
local-hostname: my-vm
```

3. **network-config** (netplan v2 format):
```yaml
version: 2
ethernets:
  eth0:
    match:
      macaddress: "be:ef:0a:37:16:16"
    addresses:
      - 10.55.22.22/24
    routes:
      - to: default
        via: 10.55.22.1
    nameservers:
      addresses: [8.8.8.8, 1.1.1.1]
```

**ISO creation:**
- Use `iso9660` library to create in-memory ISO
- Volume ID: "cidata" (required by cloud-init)
- Joliet + Rock Ridge extensions
- Upload directly to libvirt storage

### Libvirt Domain XML

Key elements (using volume-based disk sources):
```xml
<domain type='kvm'>
  <name>my-vm</name>
  <memory unit='GiB'>8</memory>
  <vcpu placement='static'>4</vcpu>
  <cpu mode='host-model'/>
  <os>
    <type arch='x86_64' machine='q35'>hvm</type>
    <loader readonly='yes' type='pflash'>/usr/share/edk2/ovmf/OVMF_CODE.fd</loader>
    <boot dev='hd'/>
  </os>
  <devices>
    <!-- Boot disk (qcow2 with backing image) -->
    <disk type='volume' device='disk'>
      <driver name='qemu' type='qcow2' cache='none'/>
      <source pool='foundry-vms' volume='my-vm_boot'/>
      <target dev='vda' bus='virtio'/>
    </disk>

    <!-- Data disks -->
    <disk type='volume' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source pool='foundry-vms' volume='my-vm_data-vdb'/>
      <target dev='vdb' bus='virtio'/>
    </disk>

    <!-- Cloud-init ISO -->
    <disk type='volume' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source pool='foundry-vms' volume='my-vm_cloudinit'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>

    <interface type='bridge'>
      <mac address='be:ef:0a:37:16:16'/>
      <source bridge='br0'/>
      <model type='virtio'/>
    </interface>
    <serial type='pty'>
      <target type='isa-serial' port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
  </devices>
</domain>
```

## CLI Interface

### Commands

**VM Management:**
```bash
# Create VM from config
foundry create <config.yaml>
foundry create examples/simple-vm.yaml
foundry create vm.yaml --pool foundry-ssd  # Use custom pool

# Destroy VM
foundry destroy <vm-name>
foundry destroy my-vm

# List all VMs
foundry list
foundry list --all  # Include stopped VMs

# Show VM details
foundry vm info <vm-name>
```

**Storage Pool Management:**
```bash
# List all pools
foundry pool list

# Show pool details (capacity, allocation, volumes)
foundry pool info <pool-name>

# Add custom pool
foundry pool add <name> <type> <path>
foundry pool add my-pool dir /mnt/ssd/foundry

# Delete custom pool (prevents deleting foundry-images, foundry-vms)
foundry pool delete <name> [--force]

# Refresh pool state
foundry pool refresh <name>
```

**Base Image Management:**
```bash
# List available base images
foundry image list

# Import local image file
foundry image import <source-path> <name>
foundry image import /path/to/fedora-43.qcow2 fedora-43.qcow2

# Download and import image from URL (TODO: not yet implemented)
foundry image pull <url> --name <image-name> [--checksum sha256:...]

# Delete base image
foundry image delete <image-name>

# Show image details and usage
foundry image info <image-name>
```

**Storage Overview:**
```bash
# Show storage status across all pools
foundry storage status
```

### Exit Codes

- 0: Success
- 1: Configuration error
- 2: Validation error
- 3: Libvirt connection error
- 4: VM operation error
- 5: Storage error

### Output Format

**Create:**
```
Creating VM 'my-vm'...
✓ Configuration validated
✓ Storage created (50 GB boot + 300 GB data)
✓ Cloud-init ISO generated
✓ Domain defined
✓ VM started
Successfully created VM 'my-vm'
```

**Destroy:**
```
Destroying VM 'my-vm'...
✓ VM stopped
✓ Domain undefined
✓ Storage removed
Successfully destroyed VM 'my-vm'
```

**List:**
```
NAME       STATE     VCPUS  MEMORY  IP ADDRESS
my-vm      running   4      8 GiB   10.20.30.40
test-vm    running   2      4 GiB   10.20.30.41
old-vm     shut off  4      8 GiB   -
```

## Phase 1 Scope (MVP)

**In Scope:**
- ✅ Create VMs with bridge networking
- ✅ Destroy VMs with full cleanup
- ✅ List VMs
- ✅ Cloud-init ISO generation
- ✅ QCOW2 snapshot boot disks
- ✅ Multiple data disks
- ✅ Multiple network interfaces
- ✅ UEFI boot
- ✅ Serial console

**In Progress - Storage Architecture:**
- 🔄 Storage pool management (foundry-images, foundry-vms pools)
- 🔄 Base image management (import, pull, list, delete)
- 🔄 Multi-pool support (custom pools on different storage backends)
- 🔄 Volume-based disk management (libvirt native)

**Out of Scope (Future Phases):**
- ❌ BGP/ethernet mode networking
- ❌ Remote hypervisor support (SSH)
- ❌ VNC configuration
- ❌ Installation ISO attachment
- ❌ CoreOS/Ignition support (provisioning format abstraction needed)
- ❌ Console autologin
- ❌ Custom firmware config (fw_cfg)
- ❌ Config file support (~/.config/foundry/config.yaml)
- ❌ Bridge verification before deployment (fuzzy match existing bridges)

## Testing Strategy

### Testing Philosophy

**Focus on Interface Contracts**: Tests should validate behavior through public APIs rather than implementation details.

**Pragmatic Coverage**: For methods that only return error/nil, test both paths. Add more specific tests only when there are meaningful behavioral variations.

**Evolutionary Testing**: Start with focused tests covering the interface contract. If specific problems arise in production:
1. Add targeted tests for those scenarios
2. Improve abstractions to make issues more testable
3. Refine interfaces based on real-world usage

This keeps tests maintainable while providing confidence in the system.

### Unit Tests
- MAC calculation from IP
- YAML config parsing
- Cloud-init template generation
- XML domain generation
- VM creation orchestration (using mocks)
- Cleanup behavior (error paths and best-effort cleanup)

### Integration Tests
- Requires libvirt running
- Create/destroy/list operations
- Storage volume operations
- Cloud-init ISO creation

### Manual Testing
- Create VM and verify it boots
- SSH into VM with injected keys
- Verify network connectivity
- Verify data disks are available
- Destroy VM and verify cleanup

## Error Handling

### Graceful Failures
- VM creation fails → clean up partial resources
- Storage creation fails → delete created volumes
- Domain definition fails → clean up storage
- Shutdown timeout → force destroy

### User-Friendly Messages
- Show progress during long operations
- Explain why validation failed
- Suggest fixes for common errors
- Show libvirt error details only with --verbose

## Future Enhancements (Phase 2+)

### Phase 2: Configuration & Validation Improvements
1. **Configurable Storage Base Path**
   - CLI flag: `--storage-path /custom/path`
   - Environment variable: `FOUNDRY_STORAGE_PATH`
   - Config file support

2. **Bridge Verification**
   - Check bridge exists on hypervisor before deployment
   - Fuzzy match bridge names
   - Suggest available bridges on error

3. **Provisioning Format Abstraction**
   - Generic provisioning config (not cloud-init specific)
   - Support cloud-init format (current)
   - Support Ignition format (CoreOS/Fedora CoreOS)
   - Adapter pattern for format conversion

### Phase 3: Remote Hypervisors
1. **Remote Connection Support**
   - Support `qemu+ssh://` URIs
   - Connection pooling
   - Parallel operations across hosts

### Phase 4: Advanced Networking
1. **BGP Networking**
   - Ethernet mode interfaces
   - BIRD integration
   - Libvirt hook management

### Phase 5: Image Management
1. **Image Operations**
   - Download images from URLs
   - Verify checksums
   - Image catalog/templates
   - Image versioning

### Phase 6: Advanced Features
1. **Console & Installation**
   - VNC console access
   - Installation ISO support
   - Custom CPU topology
   - NUMA configuration

2. **Usability**
   - Interactive VM creation wizard
   - VM templates
   - Bulk operations
   - VM migration between hosts

3. **Observability**
   - Detailed logging
   - Metrics export
   - Status monitoring
   - Event notifications

## Migration from Ansible

### Compatibility
- Config format inspired by Ansible but simplified
- MAC calculation algorithm matches exactly
- Storage layout compatible
- Can manage VMs created by Ansible

### Migration Path
1. Install foundry on hypervisor
2. Export existing VM configs to foundry YAML format
3. Test create/destroy on non-critical VMs
4. Gradually replace Ansible playbook calls with foundry
5. Update justfile to call foundry instead of ansible-playbook

### Coexistence
- Foundry and Ansible can coexist
- Both manage libvirt domains
- No state file conflicts
- Eventual goal: Replace Ansible for VM management

## Implementation Notes

### go-libvirt Library Quirks

When working with `github.com/digitalocean/go-libvirt`, be aware of these implementation details:

#### OptString Type
The `libvirt.OptString` type is defined as `type OptString []string` (a string slice), not a string wrapper.

**Usage:**
```go
// Correct - create with slice literal syntax
libvirt.OptString{"value"}
libvirt.OptString{MetadataNamespace}

// Access - use slice index
if len(optString) > 0 {
    value := optString[0]
}
```

**Common mistake:**
```go
// WRONG - doesn't compile
optString.String  // no such field or method
```

**Impact on testing:**
When creating mocks for `LibvirtClient` interface methods that use `OptString` parameters, extract values using slice indexing:

```go
func (m *mockLibvirtClient) DomainSetMetadata(
    dom libvirt.Domain,
    typ int32,
    metadata libvirt.OptString,
    key libvirt.OptString,
    uri libvirt.OptString,
    flags libvirt.DomainModificationImpact,
) error {
    // Extract values safely
    if len(metadata) > 0 {
        m.lastSetMetadata = metadata[0]
    }
    if len(key) > 0 {
        m.lastSetKey = key[0]
    }
    // ...
}
```

#### Domain Type
The `libvirt.Domain` type is a simple struct, not an interface. This makes it easy to use in tests without complex mocking.

#### Error Handling
Libvirt errors are returned as Go errors. Check specific error conditions using string matching when needed:
```go
_, err := l.DomainGetMetadata(...)
if err != nil && strings.Contains(err.Error(), "not found") {
    // Handle "not found" case
}
```

### YAML/XML Marshaling Edge Cases

When storing VM metadata using `yaml.Marshal()` and `xml.Marshal()`:

- **Nil values**: `yaml.Marshal(nil)` returns `"null\n"` without error
- **Empty structs**: Both marshalers handle empty structs gracefully
- **Marshal errors**: Practically impossible to trigger for well-formed Go structs
- **Test coverage**: Don't worry about covering marshal error paths for standard types

This is why the `metadata.Store()` function achieves only 81.8% coverage - the error paths for YAML/XML marshaling failures (lines 51, 63 in storage.go) are unreachable in practice with valid struct types.

### Embedded Field Access Pattern

The `VirtualMachine` type embeds `TypeMeta` and `ObjectMeta`, allowing direct access to their fields without qualification.

**Correct usage:**
```go
vm := &v1alpha1.VirtualMachine{}
vm.Name = "my-vm"                    // Direct access
vm.Labels = map[string]string{...}   // Direct access
vm.Annotations = map[string]string{...}
vm.Generation = 1
```

**Incorrect usage (triggers linter warning QF1008):**
```go
vm.ObjectMeta.Name = "my-vm"         // Redundant embedded field qualifier
vm.ObjectMeta.Labels = ...           // Triggers staticcheck warning
```

**Rationale:**
- Go's embedded fields promote their methods and exported fields to the embedding struct
- Accessing through the embedded field name is redundant and flagged by `staticcheck`
- This pattern applies to all embedded fields in the codebase

**In tests:**
```go
// Good
if loadedVM.Name != originalVM.Name {
    t.Errorf("Name mismatch: expected %q, got %q", originalVM.Name, loadedVM.Name)
}

// Bad - triggers QF1008
if loadedVM.ObjectMeta.Name != originalVM.ObjectMeta.Name {
    t.Errorf("Name mismatch...")
}
```

## Code Architecture

### Interface Boundaries

Foundry follows **consumer-side interface design** (the Go idiom) where interfaces are defined by the consumer package, not the provider. This pattern is used throughout the Go standard library, Kubernetes client-go, and Prometheus.

**Consumer-Side Interfaces (exported for dependency injection):**
- **`storage.LibvirtClient`** (18 methods) - Storage operations needed by storage.Manager
- **`metadata.LibvirtClient`** (2 methods) - Metadata operations needed by metadata.Client
- **`vm.LibvirtClient`** (12 methods) - VM operations needed by vm package functions

**Design Rationale:**
- **Consumer-driven**: Each package defines only the libvirt operations it actually uses
- **Interface Segregation**: Zero overlap between storage, metadata, and vm interfaces
- **Testing simplicity**: Mocks only implement needed methods (no unused interface pollution)
- **Exported for DI**: Interfaces are exported (capitalized) so constructors can accept them across packages
- **Kubernetes pattern**: Matches the pattern used in k8s client-go (e.g., `PodInterface`)

**Example Pattern:**
```go
// Package metadata defines its own interface (consumer-side)
package metadata

type LibvirtClient interface {  // Exported for dependency injection
    DomainSetMetadata(...) error
    DomainGetMetadata(...) (string, error)
}

type Client struct {
    client LibvirtClient  // Unexported field, exported type
}

func NewClient(client LibvirtClient) *Client {  // Constructor accepts interface
    return &Client{client: client}
}

// Production: NewClient(realLibvirtConnection) - implicit satisfaction
// Tests: NewClient(mockClient) - implicit satisfaction
```

**Why not repository pattern?**
The current thin-wrapper approach directly exposes `go-libvirt` types, which may seem leaky. However:
- It's pragmatic for a CLI tool (no need for abstraction layers)
- XML generation already lives in dedicated functions, not scattered
- Migration to repository pattern deferred until K8s controller work (if ever needed)

### Naming Conventions

Infrastructure-level resource naming lives in `internal/naming/`:
- **`MACFromIP(ip)`** - Calculate MAC address from IP (RFC 2731 local assignment prefix: `be:ef:XX:XX:XX:XX`)
- **`InterfaceNameFromIP(ip)`** - Calculate tap interface name (`vm{hex}`, e.g., `vm0a371616`)
- **`VolumeNameBoot(vmName)`** - Boot disk volume name (`{vmName}_boot`)
- **`VolumeNameData(vmName, device)`** - Data disk volume name (`{vmName}_data-vdb`)
- **`VolumeNameCloudInit(vmName)`** - Cloud-init ISO name (`{vmName}_cloudinit`)

**Why `internal/naming/`?**
- Infrastructure concerns (encoding rules, not business logic)
- Version-independent (won't change across API versions like v1alpha2)
- Not part of public API contract
- Previously duplicated across 3 packages - now consolidated

### Package Responsibilities

| Package | Purpose | Key Types/Functions |
|---------|---------|-------------------|
| `api/v1alpha1/` | K8s-style API types | `VirtualMachine`, `VirtualMachineSpec`, `VirtualMachineStatus` |
| `cmd/foundry/` | CLI commands | `create`, `destroy`, `list`, `get`, `pool`, `image`, `storage` |
| `internal/vm/` | VM orchestration + consumer-side interface | `Create()`, `Destroy()`, `List()`, `Get()`, `LibvirtClient` interface |
| `internal/storage/` | Storage management + consumer-side interface | `Manager`, `LibvirtClient` interface - pool/volume CRUD, image import |
| `internal/metadata/` | VM spec persistence + consumer-side interface | `Client`, `LibvirtClient` interface - persist specs in libvirt metadata |
| `internal/naming/` | Resource naming conventions | MAC/interface/volume naming functions |
| `internal/cloudinit/` | Cloud-init generation | `GenerateUserData()`, `GenerateNetworkConfig()`, `GenerateISO()` |
| `internal/libvirtxml/` | Libvirt domain XML generation | `GenerateDomainXML()` - creates libvirt XML from VirtualMachine spec |
| `internal/status/` | Status/condition management | `SetCondition()`, `SetPhase()` - K8s-style status updates |
| `internal/output/` | Output formatters | `TableFormatter`, `YAMLFormatter`, `JSONFormatter` |
| `internal/loader/` | YAML loading/validation | `LoadFromFile()`, `SaveToFile()` |

**Dependency Flow** (following Clean Architecture principles):
```
cmd/foundry/              → (uses)
internal/vm/              → (uses, defines LibvirtClient interface)
internal/storage/         → (uses, defines LibvirtClient interface)
internal/metadata/        → (uses, defines LibvirtClient interface)
github.com/digitalocean/go-libvirt  → (satisfies all interfaces implicitly)
```

All packages depend on `api/v1alpha1/` for domain types. No circular dependencies.
Each consumer package defines its own `LibvirtClient` interface with only the methods it needs.

### Testing Approach

**Interface-Based Dependency Injection (Consumer-Side Pattern):**
- Each package defines its own interface for only the operations it needs
- Constructors accept the exported interface type (e.g., `storage.LibvirtClient`)
- Production code passes `*libvirt.Libvirt` which implicitly satisfies all interfaces
- Test code passes package-specific mocks that implement the interface
- No test-only constructors needed - standard constructors work for both prod and test

**Coverage Targets:**
- Pure functions (naming, cloudinit): 100%
- Domain logic (vm, status): 85%+
- Infrastructure adapters (storage, metadata): 80%+

**Example - Testing Storage Manager:**
```go
func TestCreateVolume(t *testing.T) {
    mockClient := &mockLibvirtClient{}  // Implements storage.LibvirtClient
    mgr := storage.NewManager(mockClient)  // Same constructor as production!

    err := mgr.CreateVolume(ctx, "pool", spec)
    // Assert storage logic without real libvirt
}
```

**Example - Testing Metadata Client:**
```go
func TestStoreVM(t *testing.T) {
    mockClient := &mockLibvirtClient{}  // Implements metadata.LibvirtClient
    client := metadata.NewClient(mockClient)  // Same constructor as production!

    err := client.Store(domain, vm)
    // Assert metadata persistence without real libvirt
}
```

This architecture enables:
- Fast unit tests (no libvirt required)
- Integration tests when needed (real libvirt connection)
- No duplication between prod and test constructors
- Clean dependency injection following Go idioms
