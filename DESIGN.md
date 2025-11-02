# Plow Design Document

## Overview

Plow is a Go-based CLI tool for managing libvirt VMs, replacing the Ansible-based workflow in the homestead project. It provides simple commands to create, destroy, and list VMs using YAML configuration files.

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
plow/
├── cmd/plow/
│   └── main.go              # CLI entrypoint with subcommands
├── internal/
│   ├── config/
│   │   └── types.go         # VM configuration structs + MAC calculation
│   ├── disk/
│   │   └── storage.go       # Disk creation via libvirt storage APIs
│   ├── cloudinit/
│   │   ├── generator.go     # Generate user-data, meta-data, network-config
│   │   └── iso.go           # Create ISO using iso9660
│   ├── libvirt/
│   │   ├── client.go        # Libvirt connection management
│   │   └── domain.go        # Domain XML generation & operations
│   └── vm/
│       ├── create.go        # VM creation orchestration
│       ├── destroy.go       # VM destruction logic
│       └── list.go          # VM listing
├── examples/
│   ├── simple-vm.yaml       # Basic VM config example
│   ├── multi-disk-vm.yaml   # VM with data disks
│   └── no-cloudinit-vm.yaml # VM without cloud-init
├── go.mod
├── go.sum
├── DESIGN.md                # This file
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

### Storage Management

**Using libvirt storage APIs:**
- Create storage pool at `/var/lib/libvirt/images/`
- Each VM gets a subdirectory
- Boot disk is qcow2 volume with backing file reference
- Data disks are standalone qcow2 volumes
- Volumes managed via libvirt (no qemu-img needed)

**Storage structure:**
```
/var/lib/libvirt/images/       # Hardcoded base path (configurable in future)
├── fedora-42.qcow2            # Base images (not managed by plow)
├── ubuntu-24.04.qcow2
├── my-vm/                     # VM directory
│   ├── boot.qcow2             # Boot disk (backing: ../fedora-42.qcow2)
│   ├── data-vdb.qcow2         # Data disk
│   ├── data-vdc.qcow2         # Data disk
│   └── cloudinit.iso          # Cloud-init config
└── prod-web01/
    ├── boot.qcow2
    ├── data-vdb.qcow2
    └── cloudinit.iso
```

**File naming patterns:**
- Boot disk: `<vm-name>/boot.qcow2`
- Data disks: `<vm-name>/data-<device>.qcow2` (e.g., data-vdb, data-vdc)
- Cloud-init ISO: `<vm-name>/cloudinit.iso`

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

Key elements:
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
    <disk type='volume' device='disk'>
      <driver name='qemu' type='qcow2' cache='none'/>
      <source pool='default' volume='my-vm/boot.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='volume' device='cdrom'>
      <source pool='default' volume='my-vm/cloudinit.iso'/>
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

```bash
# Create VM from config
plow create <config.yaml>
plow create examples/simple-vm.yaml

# Destroy VM
plow destroy <vm-name>
plow destroy my-vm

# List all VMs
plow list
plow list --all  # Include stopped VMs

# Show VM details (future)
plow show <vm-name>
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

**Out of Scope (Future Phases):**
- ❌ BGP/ethernet mode networking
- ❌ Image management/downloading
- ❌ Remote hypervisor support (SSH)
- ❌ VNC configuration
- ❌ Installation ISO attachment
- ❌ CoreOS/Ignition support (provisioning format abstraction needed)
- ❌ Image templates/catalog
- ❌ Console autologin
- ❌ Custom firmware config (fw_cfg)
- ❌ Configurable storage base path (CLI flag, env var, config file)
- ❌ Bridge verification before deployment (fuzzy match existing bridges)

## Testing Strategy

### Unit Tests
- MAC calculation from IP
- YAML config parsing
- Cloud-init template generation
- XML domain generation

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
   - Environment variable: `PLOW_STORAGE_PATH`
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
1. Install plow on hypervisor
2. Export existing VM configs to plow YAML format
3. Test create/destroy on non-critical VMs
4. Gradually replace Ansible playbook calls with plow
5. Update justfile to call plow instead of ansible-playbook

### Coexistence
- Plow and Ansible can coexist
- Both manage libvirt domains
- No state file conflicts
- Eventual goal: Replace Ansible for VM management
