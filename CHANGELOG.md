# Changelog

All notable changes to foundry are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and foundry follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The version is the latest tag `vX.Y.Z`; there is no
version file. Each change adds its line under `[Unreleased]`; a release moves them under its own heading,
dated, in the commit that gets the tag. While the version is below 1.0.0, a breaking change bumps the minor
version. Releases up to 0.6.1 came before this file: their changes are in the GitHub release notes and the
git history.

## [Unreleased]

### Added

- `--socket` picks the libvirt socket for every command, for example one forwarded over SSH from a libvirt on
  another machine. `test-conn` prints the socket it used.
- `spec.arch`: x86_64 and aarch64 guests. A guest of the hypervisor's architecture runs under KVM, any other
  under QEMU emulation; aarch64 guests use the virt machine. The CPU mode default follows the domain type
  (host-model for x86_64 under KVM, host-passthrough for aarch64 under KVM, maximum under emulation). `render`
  takes `--host-arch`.
- `spec.firmware`: a pinned UEFI loader and varstore template instead of libvirt's auto-selection, with
  `secureBoot` (x86_64 only, as it needs SMM) and `keys`, a Secure Boot key profile: a varstore with its own
  PK, KEK, db and dbx. `foundry varstore import`, `list` and `delete` manage the profiles.
- `spec.serial` and `spec.smbios`: the VM's SMBIOS system information, serial included. With `spec.smbios`
  and no serial, the serial is `foundry-<name>`. A set `spec.smbios.uuid` is also the domain's UUID.
- `spec.fwCfg`: QEMU fw_cfg entries under `opt/`, each an inline string or a file on the hypervisor.
- `spec.inputDisks`: small read-only ISO-9660 disks with files for the guest, inline or read from files,
  attached as CD-ROMs after the cloud-init seed; the guest finds each by its label.
- `bootDisk.serial` and `dataDisks[].serial`: a disk serial, so the guest finds the disk as
  `/dev/disk/by-id/virtio-<serial>` whatever order it probes the disks in.
- `networkInterfaces[].hostDevice`: a fixed name for the interface's host-side device instead of the one
  derived from its IP.
- `networkInterfaces[].vlanTrunk`: an interface that is a VLAN trunk, carrying the listed 802.1Q IDs tagged
  into the guest (libvirt 11.0.0 or later for a Linux bridge). Exclusive with `vlan` and with `pxeBoot`.
- `foundry image import - <name>` imports an image from standard input; a named pipe as the path is read the
  same way. The stream is spooled to `$TMPDIR`, which needs room for one copy.
- `spec.bootDisk.imageSHA256`: the sha256 of the VM's image. Create reads the volume back and refuses one
  that differs, before it makes anything.
- `make fuzz` runs each fuzz target for `FUZZTIME` (default 20s): the spec loader, the spec stored in the
  domain's metadata, and the qcow2 header check.
- An integration lane: `make test-integration` runs foundry against the libvirt whose socket
  `FOUNDRY_INTEGRATION_SOCKET` names, and fails when no test ran. The GitHub Actions workflow `Integration`
  runs it on a GitHub-hosted runner, started by hand. See README, Integration Tests.
- `foundry destroy --force` destroys a domain foundry did not make; see Changed.

### Changed

- Without `--socket`, foundry connects to virtqemud's socket when the host runs libvirt's modular daemons,
  else to `libvirt-sock`, as libvirt's own clients do. This avoids virtproxyd, which under SELinux may refuse
  a confined caller such as a service or a container.
- An image import streams the file into the volume instead of reading it into memory, and a raw image's
  volume is exactly the file's size, no longer rounded up to whole GiB plus one. A raw image imported earlier
  is padded: import it again to pin it with `imageSHA256`.
- An image import takes a regular file or a pipe only; a directory, a device or a terminal is refused. Any
  failure after the volume is made, Ctrl-C and SIGTERM included, deletes the partial volume.
- `make test-integration` runs the new integration tests (build tag `integration`) instead of tests whose
  names contain "Integration", of which there were none. `make vet` and `make lint` also build the
  integration tests.
- Go 1.27 is the minimum to build; CI and releases build with its newest patch release. golangci-lint
  v2.15.0.
- Dependencies: libvirt.org/go/libvirtxml 1.12008.0, go.yaml.in/yaml/v3 3.0.6.
- `foundry destroy` destroys only VMs foundry made: a domain without foundry's spec in its metadata, or whose
  spec cannot be read, is refused with nothing changed. `--force` destroys it anyway, deleting its `<vm>_...`
  volumes in foundry-vms only.

### Fixed

- The spec is stored in the domain's metadata as escaped XML text. A spec whose strings held `&` or `<` (a
  shell script in `rawUserData`) made invalid XML, so the VM started and was left without its stored spec.
- A spec holding a string of several lines that starts with a tab (in user data, an input file, a fw_cfg
  string, an annotation) is stored in the domain's metadata, and printed by `get -o yaml`, with its strings of
  several lines double-quoted. go-yaml wrote such a string as a block it could not read back, so the VM lost
  its stored spec. Found by the new fuzz target for the stored spec.
- The same holds for a string of several lines whose first line is empty, in an input file or a fw_cfg
  string: go-yaml wrote it as a block with an indentation indicator that it read back as another string
  (`"\n#"` as `""`), so the stored spec loaded as a different one. The YAML written must now read back and
  marshal to the same YAML, not only parse. Found by the fuzz target for the stored spec, now given input
  files and fw_cfg strings too.
- `foundry image import` refuses a qcow2 image with a backing file or an external data file, which would
  hand the guest the content of a file on the host, and one whose version, incompatible features or header
  extensions it cannot vouch for.
- fw_cfg names longer than QEMU's 55 characters, strings of 128 KiB or more, and strings with a comma are
  refused when the spec is loaded; each passed the define and failed the start.
- A `hostDevice` with `%`, which the kernel takes as a template for a numbered name, is refused.
- `NetworkInterfaceSpec.DeepCopy` copies `vlan` instead of sharing it with the original.
- A connect cancelled by its context closes a connection that completes late instead of leaking it.
- `foundry destroy` no longer deletes images-pool volumes by name prefix. It deleted every volume named
  `<vm>_...` in foundry-images too, and image names are free text, so destroying a VM `rhel` deleted an image
  `rhel_10.qcow2`. It now searches foundry-vms by prefix and, when the spec puts the VM's disks in another pool
  (`spec.storagePool`, foundry-images included), deletes there only the boot, cloud-init, data and input
  volumes the spec names. Volumes in a pool other than the two default ones were not deleted before; they
  are now.
- `foundry destroy` of a VM `web` leaves the volumes of another VM `web_2`, which also start with `web_`.

[Unreleased]: https://github.com/jbweber/foundry/compare/v0.6.1...HEAD
