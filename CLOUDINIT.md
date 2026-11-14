# Cloud-Init Implementation Guide

## Overview

Foundry generates cloud-init configuration to provision VMs with network settings, SSH keys, hostnames, and user accounts. This document describes the cloud-init format specifications we follow and how we implement them.

## Official Documentation

All cloud-init implementation in foundry follows the official cloud-init documentation:

- **Format Specification**: https://cloudinit.readthedocs.io/en/latest/explanation/format.html
- **Network Config v2 (Netplan)**: https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html
- **NoCloud Datasource**: https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html
- **Cloud-Config Examples**: https://cloudinit.readthedocs.io/en/latest/reference/examples.html

## Cloud-Init Datasource: NoCloud

Foundry uses the **NoCloud** datasource, which reads configuration from an ISO image attached as a CDROM device.

### Requirements

Per the NoCloud specification:

1. **ISO Volume Label**: Must be `CIDATA` (uppercase)
2. **Required Files**: The ISO must contain these files in the root directory:
   - `user-data` - Cloud-config YAML for instance configuration
   - `meta-data` - Instance metadata (must include `instance-id`)
   - `network-config` - Network configuration (optional, but we always provide it)

## File Formats

### 1. user-data (Cloud-Config)

**Format**: YAML file that **must** start with `#cloud-config` header (or other valid cloud-init format).

**Specification**: https://cloudinit.readthedocs.io/en/latest/explanation/format.html

**Purpose**: Configure the instance (hostname, users, SSH keys, passwords, packages, etc.)

**Supported Formats**:
- `#cloud-config` - YAML cloud-config (most common)
- `#!/bin/bash` or `#!/bin/sh` - Shell script
- `#include` or `#include-once` - Include external files
- `## template: jinja` - Jinja2 template
- `Content-Type: multipart/mixed` - MIME multi-part for multiple configs

**Example (Generated)**:
```yaml
#cloud-config
hostname: my-vm
fqdn: my-vm.example.com
ssh_authorized_keys:
  - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo... user@host
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host
chpasswd:
  expire: false
  list: root:$6$rounds=4096$salt$hashedpassword...
ssh_pwauth: false
output:
  all: '| tee -a /var/log/cloud-init-output.log'
```

**Example (Custom Shell Script)**:
```bash
#!/bin/bash
echo "Installing k3s..."
curl -sfL https://get.k3s.io | sh -
systemctl enable k3s
```

**Key Fields** (for generated cloud-config):
- `hostname` - Short hostname for the instance
- `fqdn` - Fully qualified domain name
- `ssh_authorized_keys` - List of SSH public keys, given at the top level, so cloud-init adds them
  for the image's **default user** (`fedora`, `ubuntu`, `cloud-user`, ... as the image's cloud-init
  configuration names it), not for root
- `chpasswd.list` - `root:<passwordHash>`: the hash sets root's password
- `chpasswd.expire` - Whether to expire passwords on first login
- `ssh_pwauth` - Enable/disable SSH password authentication

### 2. meta-data

**Format**: YAML file with instance metadata.

**Specification**: https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html

**Purpose**: Provide instance identity and metadata to cloud-init.

**Required Fields**:
- `instance-id` - **REQUIRED**. Unique identifier for the instance. Cloud-init uses this to determine if this is the first boot. If you change user-data, you must also change instance-id.

**Example**:
```yaml
instance-id: my-vm
local-hostname: my-vm
```

**Implementation Note**: `instance-id` is the VM name. A VM created again under the same name
gets a new disk from the image, whose cloud-init has never run, so it still runs on first boot.

### 3. network-config (Netplan v2)

**Format**: YAML file using Netplan version 2 format.

**Specification**: https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html

**Purpose**: Configure network interfaces with static IPs, gateways, DNS servers, and routes.

**Example**:
```yaml
version: 2
ethernets:
  eth0:
    match:
      macaddress: "be:ef:0a:14:1e:28"
    addresses:
      - 10.20.30.40/24
    nameservers:
      addresses:
        - 8.8.8.8
        - 1.1.1.1
    routes:
      - to: 0.0.0.0/0
        via: 10.20.30.1
```

**Key Fields**:
- `version` - Must be `2` for netplan format
- `ethernets.{name}` - Arbitrary name for the interface (we use eth0, eth1, etc.)
- `match.macaddress` - Match interface by MAC address (lowercase per spec)
- `addresses` - List of IP addresses in CIDR notation (e.g., `10.20.30.40/24`)
- `nameservers.addresses` - The interface's `dnsServers`, when the spec gives any
- `routes` - A default route (`to: 0.0.0.0/0`) via the interface's `gateway`, only on the
  interface with `defaultRoute: true`; the other interfaces get no route

**Multiple Interfaces**: For VMs with multiple network interfaces, add additional entries under `ethernets`:
```yaml
version: 2
ethernets:
  eth0:
    match:
      macaddress: "be:ef:0a:14:1e:28"
    addresses: [10.20.30.40/24]
    nameservers:
      addresses: [8.8.8.8]
    routes:
      - to: 0.0.0.0/0
        via: 10.20.30.1
  eth1:
    match:
      macaddress: "be:ef:c0:a8:01:32"
    addresses: [192.168.1.50/24]
    nameservers:
      addresses: [192.168.1.1]
```

## Implementation Details

### ISO Generation

Foundry creates the cloud-init ISO using the `github.com/kdomanski/iso9660` pure Go library:

1. Create an in-memory ISO9660 filesystem
2. Add three files: `user-data`, `meta-data`, `network-config`
3. Set volume label to `CIDATA`
4. Write it as plain ISO-9660, without Rock Ridge or Joliet: the three file names pass through
   unchanged, and cloud-init reads them as they are
5. Upload the ISO to libvirt storage
6. Attach the ISO as a CDROM device (SATA bus) to the VM

### MAC Address Calculation

Network interface MAC addresses are deterministically calculated from the IP address:

**Algorithm** (matching Ansible implementation):
```
IP: 10.20.30.40
Convert octets to hex: 0a 14 1e 28
MAC: be:ef:0a:14:1e:28
```

This ensures:
- Deterministic MAC addresses from IP
- Unique MACs per interface
- Compatibility with existing homestead VMs
- Easy matching in network-config via `macaddress` field

### Instance ID Generation

Format: `{vm-name}`

Example: `my-vm`

**Why**: The `instance-id` tells cloud-init whether this is a first boot. Each create gives the VM
a new boot disk from its image, on which cloud-init has not run, so the name is enough.

### User Data Generation

Foundry supports two modes for user-data generation:

#### 1. Generated User-Data (Default)

When no raw user-data is provided, foundry generates a standard cloud-config:

1. Start with `#cloud-config` header (required)
2. Marshal the configuration to YAML
3. Set `hostname` and `fqdn` from config
4. Add `ssh_authorized_keys` array from config
5. If a password hash is provided, add a `chpasswd` section setting root's password
6. Set `ssh_pwauth` to false (unless explicitly enabled)

**Important**: The `#cloud-config` header must be the first line, followed by valid YAML.

#### 2. Custom Raw User-Data

For advanced use cases (installing software, running custom scripts, etc.), you can provide complete custom user-data via the `rawUserData` field in the CloudInit spec:

```yaml
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: k3s-server
spec:
  vcpus: 4
  memoryGiB: 8
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  cloudInit:
    rawUserData: |
      #!/bin/bash
      curl -sfL https://get.k3s.io | sh -
      systemctl enable k3s
  networkInterfaces:
    - ip: 10.250.250.100/24
      gateway: 10.250.250.1
      bridge: br0
      defaultRoute: true
```

**When using `rawUserData`**:
- The content is validated but used as-is
- Must start with a valid cloud-init header (`#cloud-config`, `#!/`, `#include`, etc.)
- Other CloudInit fields (FQDN, SSH keys, password) are **ignored**
- Meta-data and network-config are **still generated automatically**

**Use Cases for Raw User-Data**:
- Installing software at boot time (k3s, k8s, docker, etc.)
- Running complex initialization scripts
- Using advanced cloud-config modules (packages, runcmd, write_files, etc.)
- MIME multi-part configurations combining multiple formats

## Cloud-Init Execution Flow

When the VM boots with the cloud-init ISO attached:

1. Cloud-init detects the `CIDATA` labeled volume (NoCloud datasource)
2. Reads `meta-data` to get `instance-id` and determine if first boot
3. Reads `network-config` and applies network settings before `user-data` runs
4. Reads `user-data` and executes cloud-config modules:
   - Set hostname/FQDN
   - Create/configure users
   - Add SSH keys
   - Set passwords (if configured)
   - Many other possible modules
5. Marks instance as configured (stores instance-id for future boots)

On subsequent boots, cloud-init checks the `instance-id` against stored value. If unchanged, most configuration is skipped (modules run "once per instance" by default).

## Configuration Validation

The loader checks, when the spec is loaded (see DESIGN.md, Validation checks):

1. **Required fields present**: at least one network interface, each with `ip`, `gateway` and `bridge`
2. **Network config validity**: `ip` is an IPv4 address with a prefix length, `gateway` an IP
   address, and no address is used twice
3. **Password hash**: a single line (cloud-init passes it to `chpasswd` as one line)
4. **rawUserData**: starts with a valid cloud-init header (`#cloud-config`, `#!/`, `#include`,
   `## template:`, `Content-Type:`), checked when the seed is generated

Not checked: the format of SSH keys, of DNS server addresses, and whether a password hash is a
crypt hash. Whether the image exists is checked when the VM is created; whether the bridge exists
is not checked by foundry (libvirt fails the start).

## Testing Cloud-Init

### Manual Testing

After creating a VM with cloud-init:

1. **Check cloud-init status** (as the image's default user, which has the SSH keys, such as
   `fedora` or `cloud-user`):
   ```bash
   ssh fedora@vm-ip
   cloud-init status --long
   ```

2. **View cloud-init logs**:
   ```bash
   cat /var/log/cloud-init.log
   cat /var/log/cloud-init-output.log
   ```

3. **Verify network configuration**:
   ```bash
   ip addr show
   ip route show
   cat /etc/resolv.conf
   ```

4. **Verify user-data was applied**:
   ```bash
   hostname
   hostname --fqdn
   cat ~/.ssh/authorized_keys
   ```

5. **Check instance-id**:
   ```bash
   cat /var/lib/cloud/data/instance-id
   ```

### Re-running Cloud-Init

To test cloud-init changes, you can force re-run on an existing VM:

```bash
# Clean cloud-init state and reboot
sudo cloud-init clean
sudo reboot
```

**Warning**: This will re-run all cloud-init modules as if it's a first boot.

## Limitations and Future Work

### Current Limitations

1. **NoCloud only**: We only support NoCloud datasource (ISO-based)
2. **Static network only**: Only static IP configuration (no DHCP)
3. **No vendor-data**: We don't generate vendor-data (optional in NoCloud)

### Future Enhancements

- Support DHCP network configuration
- Support for ConfigDrive datasource
- Support for CoreOS Ignition (alternative to cloud-init)
- User-data templating with variable substitution

## References

- **Cloud-init official docs**: https://cloudinit.readthedocs.io/
- **Netplan documentation**: https://netplan.io/reference
- **NoCloud datasource**: https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html
- **ISO9660 Go library**: https://github.com/kdomanski/iso9660
- **Foundry design document**: [DESIGN.md](DESIGN.md)

## Version History

- **2025-11-07**: Added support for custom raw user-data (all cloud-init formats)
- **2025-11-02**: Initial documentation created, following cloud-init specs for foundry implementation
