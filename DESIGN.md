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
│   ├── pool.go              # Pool management commands
│   ├── image.go             # Image management commands
│   └── storage.go           # Storage status command
├── internal/
│   ├── config/
│   │   └── types.go         # VM configuration structs + MAC calculation
│   ├── storage/
│   │   ├── types.go         # Storage types (PoolType, VolumeSpec, etc.)
│   │   ├── manager.go       # Storage manager coordinator
│   │   ├── pool.go          # Pool operations (create, list, delete)
│   │   ├── volume.go        # Volume operations (create, delete, upload)
│   │   └── image.go         # Base image management (import, pull, list)
│   ├── cloudinit/
│   │   ├── generator.go     # Generate user-data, meta-data, network-config
│   │   └── iso.go           # Create ISO using iso9660
│   ├── libvirt/
│   │   ├── client.go        # Libvirt connection management
│   │   └── domain.go        # Domain XML generation & operations
│   └── vm/
│       ├── create.go        # VM creation orchestration
│       ├── destroy.go       # VM destruction logic
│       ├── list.go          # VM listing
│       └── interfaces.go    # Interfaces for dependency injection
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

```yaml
# Required fields
name: my-vm                   # VM name (libvirt domain name, normalized to lowercase)
vcpus: 4                      # Number of virtual CPUs
memory_gib: 8                 # Memory in GiB

# Boot disk configuration
boot_disk:
  size_gb: 50                 # Disk size in GB
  image: /var/lib/libvirt/images/fedora-42.qcow2  # Base image path
  format: qcow2               # Optional: qcow2 (default), raw
  # OR for empty boot disk:
  # empty: true               # Create empty disk instead of snapshot

# Optional: Additional data disks
data_disks:
  - device: vdb               # Device name (vdb, vdc, etc.)
    size_gb: 100
  - device: vdc
    size_gb: 200

# Network configuration
network_interfaces:
  - ip: 10.20.30.40/24        # IP with CIDR
    gateway: 10.20.30.1       # Gateway IP
    dns_servers:              # DNS servers
      - 8.8.8.8
      - 1.1.1.1
    bridge: br0               # Bridge name to attach to
    default_route: true       # Set default route (optional, default: true for first interface)

  # Optional: Additional interfaces
  - ip: 192.168.1.50/24
    gateway: 192.168.1.1
    dns_servers:
      - 192.168.1.1
    bridge: br1
    default_route: false

# Optional: Cloud-init configuration
cloud_init:
  fqdn: my-vm.example.com     # FQDN (hostname derived from this, normalized to lowercase)
  ssh_keys:                   # SSH public keys to inject
    - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo..."
    - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC..."
  root_password_hash: "$6$..."  # Optional: Root password hash
  ssh_pwauth: false           # Optional: Enable SSH password auth (default: false)

# Optional: Advanced settings
cpu_mode: host-model          # CPU mode: host-model (default), host-passthrough
autostart: true               # Auto-start VM on host boot (default: true)
storage_pool: foundry-vms     # Storage pool to use (default: foundry-vms)
```

### Configuration Validation Rules

**Required:**
- `name`, `vcpus`, `memory_gib`
- `boot_disk.size_gb`
- `boot_disk.image` OR `boot_disk.empty: true`
- At least one `network_interfaces` entry with `ip`, `gateway`, `bridge`

**Normalization (automatic):**
- `name` → lowercase
- `cloud_init.fqdn` → lowercase (hostname derived from this)

**Validation checks:**
- `name` format: `^[a-z0-9][a-z0-9_-]*[a-z0-9]$` (after normalization)
  - Must start and end with alphanumeric
  - Can contain alphanumeric, hyphens, underscores
- `cloud_init.fqdn` format: valid FQDN (hostname + domain with dots)
- VCPUs > 0, memory_gib > 0, disk sizes > 0
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
foundry pool add <name> --type dir --path /mnt/ssd/foundry

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
foundry image import <file-path> --name <image-name>

# Download and import image from URL
foundry image pull <url> --name <image-name> [--checksum sha256:...]

# Delete base image (checks if in use)
foundry image delete <image-name> [--force]

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
