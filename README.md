# Plow

A Go-based CLI tool for managing libvirt VMs with simple YAML configuration.

## Overview

Plow provides a straightforward way to create, manage, and destroy libvirt virtual machines using declarative YAML configuration files. It's designed to replace complex Ansible workflows with a simple, fast CLI tool.

## Features

- **Simple Configuration**: Define VMs in easy-to-read YAML files
- **Pure Go Implementation**: No CGo dependencies, easy to install and deploy
- **Cloud-init Support**: Automatic SSH key injection and network configuration
- **Bridge Networking**: Support for multiple network interfaces with bridge connectivity
- **Storage Management**: QCOW2 boot disks with backing images, plus additional data disks
- **UEFI Boot**: Modern UEFI firmware support
- **Deterministic MACs**: MAC addresses automatically calculated from IP addresses

## Installation

### Prerequisites

- Go 1.24 or later
- libvirt/libvirtd running locally
- QEMU/KVM installed

### Build from Source

```bash
# Clone the repository
git clone https://github.com/jbweber/plow.git
cd plow

# Build
make build

# Install to $GOPATH/bin
make install
```

## Usage

### Create a VM

```bash
plow create examples/simple-vm.yaml
```

### List VMs

```bash
plow list
plow list --all  # Include stopped VMs
```

### Destroy a VM

```bash
plow destroy my-vm
```

## Configuration

See [examples/](examples/) directory for sample configurations.

Basic VM configuration:

```yaml
name: my-vm
vcpus: 4
memory_gib: 8

boot_disk:
  size_gb: 50
  image: /var/lib/libvirt/images/fedora-42.qcow2

network_interfaces:
  - ip: 10.20.30.40/24
    gateway: 10.20.30.1
    dns_servers:
      - 8.8.8.8
    bridge: br0

cloud_init:
  enabled: true
  ssh_keys:
    - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo..."
```

For complete configuration options, see [DESIGN.md](DESIGN.md#configuration-format).

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
plow/
├── cmd/plow/           # CLI entry point
├── internal/
│   ├── config/         # Configuration types and parsing
│   ├── cloudinit/      # Cloud-init ISO generation
│   ├── disk/           # Storage management
│   ├── libvirt/        # Libvirt client and domain operations
│   ├── network/        # Network utilities (MAC calculation)
│   └── vm/             # VM creation, destruction, listing
├── examples/           # Example configurations
└── DESIGN.md          # Detailed design document
```

## Documentation

- [DESIGN.md](DESIGN.md) - Complete design specification
- [PLAN_PLOW.md](../PLAN_PLOW.md) - Implementation plan and progress
