# Foundry

[![CI](https://github.com/jbweber/foundry/workflows/CI/badge.svg)](https://github.com/jbweber/foundry/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/jbweber/foundry)](https://goreportcard.com/report/github.com/jbweber/foundry)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jbweber/foundry)](https://github.com/jbweber/foundry/blob/main/go.mod)
[![License](https://img.shields.io/github/license/jbweber/foundry)](https://github.com/jbweber/foundry/blob/main/LICENSE)
[![Release](https://img.shields.io/github/v/release/jbweber/foundry)](https://github.com/jbweber/foundry/releases)

A Go-based CLI tool for managing libvirt VMs with simple YAML configuration.

## Overview

Foundry provides a straightforward way to create, manage, and destroy libvirt virtual machines using declarative YAML configuration files. It's designed to replace complex Ansible workflows with a simple, fast CLI tool.

This project isn't suggested for general purpose use, and really exists because of my desire to "roll my own" things. Generally I would suggest using Proxmox or KubeVirt instead of this. If you must though feel free to let me know!

## Features

- **Kubernetes-Style API**: Familiar `apiVersion`, `kind`, `metadata`, `spec`, `status` format
- **Status Observation**: Automatic status population with phases and conditions
- **Simple Configuration**: Define VMs in easy-to-read YAML files
- **Pure Go Implementation**: No CGo dependencies, easy to install and deploy
- **Cloud-init Support**: Automatic SSH key injection and network configuration
- **Bridge Networking**: Support for multiple network interfaces with bridge connectivity
- **VLAN Tagging**: Optional 802.1Q tag applied per interface on the bridge port
- **PXE Boot**: Optionally network boot an interface ahead of the boot disk
- **Storage Management**: QCOW2 boot disks with backing images, plus additional data disks
- **Pool Management**: Create, list, and manage libvirt storage pools
- **Image Management**: Import, list, and manage base OS images
- **UEFI Boot**: Modern UEFI firmware support, with pinned firmware files and Secure Boot key profiles
- **SMBIOS and fw_cfg**: Optional SMBIOS system serial and identity, and QEMU fw_cfg entries for the guest
- **Input Disks**: Optional small read-only disks with files for the guest, found by their label
- **x86_64 and aarch64 Guests**: KVM when the guest matches the hypervisor's architecture, QEMU emulation otherwise
- **Deterministic MACs**: MAC addresses automatically calculated from IP addresses
- **Metadata Persistence**: VM specs stored in libvirt domain metadata for state recovery

## Installation

### Prerequisites

- libvirt/libvirtd running locally
- QEMU/KVM installed
- Go 1.27+ (only to build from source; release binaries and the container image have no Go dependency)

### From GitHub Releases

Download the latest release from the [releases page](https://github.com/jbweber/foundry/releases):

```bash
# Download and extract (replace VERSION with actual version, e.g., v0.1.0)
wget https://github.com/jbweber/foundry/releases/download/VERSION/foundry_VERSION_linux_amd64.tar.gz
tar -xzf foundry_VERSION_linux_amd64.tar.gz
sudo mv foundry /usr/local/bin/
```

### Docker

```bash
# Pull the image
docker pull ghcr.io/jbweber/foundry:latest

# Run a command
docker run --rm ghcr.io/jbweber/foundry:latest --help
```

### Build from Source

```bash
# Clone the repository
git clone https://github.com/jbweber/foundry.git
cd foundry

# Build
make build

# Install to $GOPATH/bin
make install
```

## Usage

### Create a VM

```bash
foundry create examples/vm.yaml
```

### Preview the Domain XML

Render the libvirt domain XML a config would produce, without connecting to
libvirt or creating anything:

```bash
foundry render examples/vm.yaml
```

The config is loaded, defaulted, and validated exactly as `create` does, so the
output matches what would be handed to libvirt — useful for checking boot order,
VLAN tags, and disk targets while iterating on a config. Referenced pools,
volumes, and base images are not checked for existence.

A config with `spec.arch` renders for the machine `render` runs on; use
`--host-arch aarch64` (or `x86_64`) to see what another hypervisor would get.

### Guest Architecture

`spec.arch` picks the guest architecture: `x86_64` (the default) or `aarch64`.
A guest of the hypervisor's architecture runs under KVM; any other runs under
QEMU emulation, which is much slower but lets an x86_64 host boot an aarch64
image, for example. Emulation needs the QEMU system emulator and UEFI firmware
for the guest's architecture installed on the hypervisor. A config without
`spec.arch` is an x86_64 guest under KVM, as before.

### UEFI Firmware and Secure Boot

Without `spec.firmware`, libvirt picks the UEFI firmware for the guest's
architecture, as before. `spec.firmware` pins it instead:

```yaml
spec:
  firmware:
    loader: /usr/share/edk2/ovmf/OVMF_CODE.secboot.fd  # firmware code on the hypervisor
    secureBoot: true        # secure loader and SMM; x86_64 only
    keys: my-keys           # a Secure Boot key profile, or:
    # nvramTemplate: /usr/share/edk2/ovmf/OVMF_VARS.secboot.fd
```

A key profile is a UEFI varstore with your own PK, KEK, db and dbx enrolled
(built, for example, with `virt-fw-vars` from
[virt-firmware](https://gitlab.com/kraxel/virt-firmware)), imported once per
architecture into the `foundry-images` pool:

```bash
foundry varstore import my-keys.fd my-keys                  # x86_64 (default)
foundry varstore import - my-keys --arch aarch64 < arm.fd   # from stdin
foundry varstore list
foundry varstore delete my-keys --arch aarch64
```

`create` looks up the profile's varstore for `spec.arch` and fails before
making anything if it is missing. libvirt copies the template into each VM's
own varstore on its first start, so replacing or deleting a profile leaves
existing VMs alone.

### SMBIOS and fw_cfg

A guest that binds something to its machine needs an SMBIOS system serial
that is set and stable; QEMU sets none. `spec.serial` sets one, and
`spec.smbios` sets the rest of the system information (unset fields keep
QEMU's values). With `spec.smbios` and no `spec.serial`, the serial is
`foundry-<name>`. A spec with neither gets no SMBIOS information, as before.

```yaml
spec:
  serial: SN-0001           # /sys/class/dmi/id/product_serial (default with smbios: foundry-<name>)
  smbios:                   # "smbios: {}" for just the default serial
    manufacturer: Example Corp
    product: Model 9
    version: "1.0"
    sku: SKU-42
    family: Servers
    uuid: 4c4c4544-0042-3510-8051-b4c04f4e3532  # also becomes the domain UUID
  fwCfg:                    # QEMU fw_cfg entries, read from /sys/firmware/qemu_fw_cfg/by_name/<name>/raw
    - name: opt/org.tianocore/EnableLegacyLoader
      string: "no"
    - name: opt/com.example/config
      file: /srv/config.json  # a file on the hypervisor
```

fw_cfg names are at most 55 characters (QEMU's limit), and a `string` is at most
128 KiB less 128 bytes with no comma (libvirt passes it to QEMU unescaped); larger
content, or content with commas, goes in a `file`. A `file` entry gives the
guest the content of a file on the hypervisor, and libvirt relabels the file so
QEMU can read it: only use specs from people who may read that file as root.
`smbios.uuid` can't change on an existing VM (libvirt keeps a domain's UUID);
set it at creation.

### Input Disks

`spec.inputDisks` gives the guest a few small files on read-only disks, such as
an installer's config or anything its first boot reads before it has a network.
Each entry is one ISO-9660 disk with a volume label and its files in the root
directory. foundry builds the image, uploads it as a volume of the VM's pool
(`<vm>_input-<label in lower case>.iso`) and attaches it as a CD-ROM. The guest
finds a disk by its label, not by its device name:

```yaml
spec:
  inputDisks:
    - label: NODECFG              # 1-32 of A-Z 0-9 _
      files:
        - name: node.yaml         # a-z 0-9 _ -, optional .ext of 1-8; at most 28
          file: inputs/node.yaml  # read by foundry; relative to this YAML file
        - name: install-config
          string: |               # or the content inline
            disk=/dev/vda
```

```sh
# In the guest
mount -o ro LABEL=NODECFG /mnt
```

The files of one disk may hold 16 MiB, and inline `string` content 1 MiB across
all disks (it is stored with the spec in the domain's metadata); larger content
belongs in an image. `file` must be a regular file, and foundry reads it with
the permissions of the user running foundry: only use specs from people who may
read that file as that user. `CIDATA` is the label of the cloud-init seed, so it
is refused next to `spec.cloudInit`; without `spec.cloudInit`, an input disk
labelled `CIDATA` is a hand-made NoCloud seed. A spec without `inputDisks`
gets no input disks, as before. `foundry destroy` deletes them with the VM.

### List VMs

```bash
foundry list
```

### Destroy a VM

```bash
foundry destroy my-vm
```

### Manage Images

```bash
# Import a base image
foundry image import /path/to/fedora-43.qcow2 fedora-43.qcow2

# List images
foundry image list

# Show image details
foundry image info fedora-43.qcow2

# Delete image
foundry image delete fedora-43.qcow2
```

### Manage Storage Pools

```bash
# List all storage pools
foundry pool list

# Show pool details
foundry pool info foundry-images

# Create a new pool
foundry pool add my-pool dir /var/lib/libvirt/images/my-pool

# Refresh pool (detect external changes)
foundry pool refresh my-pool

# Delete pool (use --force to delete with volumes)
foundry pool delete my-pool
```

### View Storage Status

```bash
# Show overview of all storage pools
foundry storage status
```

### Choose the libvirt Socket

Foundry talks to `qemu:///system` over a UNIX socket. By default it uses
virtqemud's socket (`/var/run/libvirt/virtqemud-sock`) when it exists, as
virsh does on hosts that run libvirt's modular daemons, and the legacy
`/var/run/libvirt/libvirt-sock` otherwise.

`--socket` picks another socket for any command. To manage a libvirt on
another machine, forward its socket over SSH and point foundry at the local
end:

```bash
ssh -N -L /tmp/libvirt.sock:/run/libvirt/virtqemud-sock user@hypervisor &
foundry --socket /tmp/libvirt.sock list
```

Domains and volumes live on the far end. Files foundry reads itself (the VM
config, an image to import) come from the machine foundry runs on. Volume
ownership comes from the local `/etc/libvirt/qemu.conf` and QEMU user, so the
QEMU user's UID and GID should match on both machines.

## Configuration

See [examples/](examples/) directory for sample configurations.

Basic VM configuration (Kubernetes-style API):

```yaml
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: my-vm
  labels:
    environment: production

spec:
  vcpus: 4
  memoryGiB: 8

  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
    imagePool: foundry-images

  networkInterfaces:
    - ip: 10.20.30.40/24
      gateway: 10.20.30.1
      dnsServers:
        - 8.8.8.8
      bridge: br0
      vlan: 100               # Optional: 802.1Q VLAN ID (1-4094)

  cloudInit:
    fqdn: my-vm.example.com
    sshAuthorizedKeys:
      - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo..."
    # Or use custom raw user-data:
    # rawUserData: |
    #   #!/bin/bash
    #   curl -sfL https://get.k3s.io | sh -

status:
  phase: Running
  conditions:
    - type: Ready
      status: "True"
```

A fully annotated example covering every field lives in [examples/vm.yaml](examples/vm.yaml). For the complete configuration reference, see [DESIGN.md](DESIGN.md#configuration-format).

## Development

### Running Tests

```bash
# All tests
make test

# Unit tests only
make test-unit

# With coverage
make coverage
```

### Linting

```bash
# Format, vet, and lint
make all

# Just lint
make lint
```

### Project Structure

```
foundry/
├── cmd/foundry/        # CLI entry point and commands
├── api/v1alpha1/       # Kubernetes-style API types (VirtualMachine)
├── internal/
│   ├── loader/         # YAML config loader for v1alpha1
│   ├── metadata/       # Libvirt metadata storage for VM specs
│   ├── status/         # Status management (phases, conditions)
│   ├── output/         # Output formatters (table, YAML, JSON)
│   ├── storage/        # Storage pool and volume management
│   ├── cloudinit/      # Cloud-init and input disk ISO generation
│   ├── libvirt/        # Libvirt client and domain operations
│   └── vm/             # VM lifecycle operations (create, destroy, list, get)
├── examples/           # Example configurations
└── DESIGN.md          # Detailed design document
```

## Documentation

- [DESIGN.md](DESIGN.md) - Complete design specification
