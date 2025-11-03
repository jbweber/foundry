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
- **Storage Management**: QCOW2 boot disks with backing images, plus additional data disks
- **Pool Management**: Create, list, and manage libvirt storage pools
- **Image Management**: Import, list, and manage base OS images
- **UEFI Boot**: Modern UEFI firmware support
- **Deterministic MACs**: MAC addresses automatically calculated from IP addresses
- **Metadata Persistence**: VM specs stored in libvirt domain metadata for state recovery

## Installation

### Prerequisites

- libvirt/libvirtd running locally
- QEMU/KVM installed

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
foundry create examples/simple-vm.yaml
```

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

  cloudInit:
    fqdn: my-vm.example.com
    sshAuthorizedKeys:
      - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFoo..."

status:
  phase: Running
  conditions:
    - type: Ready
      status: "True"
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
foundry/
├── cmd/foundry/        # CLI entry point and commands
├── api/v1alpha1/       # Kubernetes-style API types (VirtualMachine)
├── internal/
│   ├── loader/         # YAML config loader for v1alpha1
│   ├── metadata/       # Libvirt metadata storage for VM specs
│   ├── status/         # Status management (phases, conditions)
│   ├── output/         # Output formatters (table, YAML, JSON)
│   ├── storage/        # Storage pool and volume management
│   ├── cloudinit/      # Cloud-init ISO generation
│   ├── libvirt/        # Libvirt client and domain operations
│   └── vm/             # VM lifecycle operations (create, destroy, list, get)
├── examples/           # Example configurations
└── DESIGN.md          # Detailed design document
```

## Documentation

- [DESIGN.md](DESIGN.md) - Complete design specification
