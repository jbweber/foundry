# Changelog

All notable changes to foundry are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and foundry follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The version is the latest tag `vX.Y.Z`; there is no
version file. Each change adds its line under `[Unreleased]`; a release moves them under its own heading,
dated, in the commit that gets the tag. While the version is below 1.0.0, a breaking change bumps the minor
version. Releases up to 0.6.1 came before this file: their changes are in the GitHub release notes and the
git history.

## [Unreleased]

## [0.7.0] - 2026-11-14

All earlier versions, v0.1.0 to v0.6.1, are retracted: Go no longer picks them, and `go list -m -u` warns
about them. They can lose data. `foundry destroy` could delete the disks of a guest that was still running or
paused, and the disks of another VM whose name starts with the same name and `_`. A failed create could delete
volumes it did not make. And the VM's spec was lost the first time the VM stopped, after which `destroy`
refused the VM as not foundry's. Upgrade to v0.7.0; see the upgrade note under Fixed for VMs created by
earlier versions.

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
- New VM names follow a rule: 1 to 63 characters from `a-z 0-9 . -`, starting with a letter or digit
  (after lower-casing). `_` is no longer allowed, since the name followed by `_` is the prefix of the VM's
  volume names. A data disk's `device` must be `vdb` to `vdzz` (`^vd[b-z]{1,2}$`). Both are checked when a
  spec is loaded for `create` and `render`; existing VMs are listed, shown and destroyed as before,
  whatever their name.
- Volumes and pools are created without an owner or group, which libvirt sets itself while a domain uses a
  volume; foundry no longer reads the QEMU user from `/etc/libvirt/qemu.conf`, so a remote libvirt over
  `--socket` no longer needs matching QEMU UIDs.
- `spec.bootDisk.imagePool` and the `pool:volume` image form accept only `foundry-images`; any other pool is
  refused when the spec is loaded. Create always took the image from foundry-images, whatever pool the spec
  named, so a VM could boot another image than the one meant.
- Release archives, checksums and the ghcr.io image carry signed build provenance; check it with
  `gh attestation verify` (README, Verify a Release). CI pins every action by commit, runs with read-only
  permissions, and runs govulncheck (`make vuln`) on every push, pull request and weekly.
- Dependencies: golang.org/x/crypto 0.56.0 (through the lint tooling); golang.org/x/vuln 1.9.0 for
  govulncheck as a Go tool.

### Fixed

- The spec is stored in the domain's persistent definition (`VIR_DOMAIN_AFFECT_CONFIG`), right after the
  define and before the VM starts, and read back from there. It was stored after the start with libvirt's
  default, which on a running domain writes only the live definition: the spec was gone at the VM's first
  stop, and `destroy` then refused the VM as not foundry's. A spec that cannot be stored now fails the
  create, which removes the domain and the volumes it made; it was only a warning.
- The spec as stored in the domain's metadata may take at most 1.5 MiB, checked before anything is made.
  Inline content other than input disk strings (cloud-init user data, fw_cfg strings, annotations) had no
  bound, so a spec too large for libvirt to store created and started a VM without its spec.
- Upgrading: VMs created by earlier versions have their spec only in the live definition. While running they
  are now refused by `destroy` as not foundry's and shown by `list` and `get` without their spec; after
  their first stop they have no spec at all. Remove such VMs with `foundry destroy --force`, which
  deletes their volumes in foundry-vms only: volumes of a VM whose `spec.storagePool` was another pool must
  be deleted by hand (their names start with `<vm>_`, for example with `virsh vol-delete --pool <pool>
  <volume>`). Recreate the VM from its spec file to manage it with foundry again.
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
- `foundry destroy` lists the domains before it changes anything, and stops with nothing changed when it
  can't, also with `--force`. It listed them after the undefine, to spare the volumes of another VM whose name
  starts with the destroyed VM's name and `_`, and when the listing failed it only warned and went on with
  none spared: destroying `web` deleted the volumes of a VM `web_2`.
- `foundry destroy` stops the VM in every state but shut off before it undefines it: a paused, suspended or
  crashed VM is destroyed at once, any other gets the graceful shutdown first. It stopped only a running VM,
  and turned a failed forced stop, or a state it could not read, into a warning; undefining an active domain
  only makes it transient, so such a VM kept running on disks destroy had deleted. A VM that cannot be
  stopped, or whose state cannot be read, is now an error with nothing undefined or deleted, also with
  `--force`.
- A failed `foundry create` deletes only the volumes it made. It deleted every data and cloud-init volume
  name the spec gives, so a volume of such a name that existed already (a leftover, an image in the pool
  `spec.storagePool` names) was deleted. Create now also checks, before it makes anything, that none of the
  VM's boot, data and cloud-init volumes exists, as it did for the boot and input volumes, and names the one
  that does.
- A failed `foundry create` undefines its domain with its UEFI varstore (`VIR_DOMAIN_UNDEFINE_NVRAM`). Once
  a start had made the varstore, libvirt refused the plain undefine, so the domain was left defined with its
  disks deleted. The cleanup's failures are no longer only logged: the create's error says what was left
  behind. A domain that may still be running (it could not be stopped, or its state could not be read) is
  left defined with its volumes, for `foundry destroy` to remove, instead of being undefined while it runs.
- A VM's own volumes (boot and data disks, the cloud-init ISO with its password hash, input disks) are created
  0600, and the foundry-vms pool's directory 0711; they were 0644 and 0755, readable and listable by every
  local user. Base images and Secure Boot key profiles stay 0644 and other pools 0755: they are shared and
  hold nothing secret. libvirt keeps the mode of volumes and pool directories made before: tighten them by
  hand on the hypervisor, as root, with `chmod 0711` on the foundry-vms directory and `chmod 0600` on the
  VMs' volumes in it (`virsh pool-dumpxml foundry-vms` shows the path), and the same for VM volumes in
  another pool a spec's `storagePool` names.
- A boot image given as a host path (`bootDisk.image: /path/...`) is checked as `foundry image import`
  checks an image, before anything is made: a regular file holding a qcow2 or a bootable raw image, and a
  qcow2 without a backing file or an external data file, which QEMU would read from the host. Its format is
  detected from its content and named as the boot overlay's backing format; it was guessed from the file
  name, so a raw image named `.qcow2`, or one named without `.raw` or `.img`, was opened as the wrong
  format.
- The loader checks addresses and a few strings it took as given: `ip` must be an IPv4 address with a prefix
  length and `gateway` an IP address (a bad one failed only after volumes were made), duplicate addresses are
  found whatever their prefix length (`10.0.0.5/24` and `10.0.0.5/16` got the same MAC), fw_cfg names take only
  `A-Z a-z 0-9 . _ / -` after `opt/` (a comma kept the VM from starting), and `passwordHash` must be one line.
- `foundry create` refuses a boot disk smaller than its image's virtual size, before it makes anything. The
  disk was made at the smaller size without a warning, and the guest saw a disk cut off in the middle of its
  partitions.
- Ctrl-C or SIGTERM during `foundry create` stops it and removes what it made so far; during `foundry destroy` it
  stops before the domain is undefined. A second Ctrl-C ends foundry at once. Before, an interrupted create left
  volumes behind and `foundry destroy` answered that the VM did not exist; it now removes the `<vm>_...` volumes
  left in foundry-vms of a name that has no domain.
- A volume check that fails for another reason than "no such volume" (a lost connection, a stopped pool) is an
  error; it was taken as "absent", so create could go ahead over a volume that was there. `image list` and
  `image info` show the image's format, which was blank.
- `foundry image delete` refuses an image that is the backing file of a volume in any pool, such as a VM's boot
  disk, which could then no longer start, and refuses a Secure Boot key profile (`foundry varstore delete` manages
  those). It deleted both.
- `foundry pool delete --force` that cannot delete a volume says which and keeps the pool; it reported success
  and left the files behind with no pool.
- `foundry varstore import --replace` keeps the old profile when the new one cannot be written: it reads the
  old one back before deleting it and puts it back on failure. It deleted the old one first, so a failed upload
  lost it.
- DESIGN.md, README and CLOUDINIT.md describe the code as it is: every error exits 1; no `create --pool`,
  `FOUNDRY_VM_POOL`, config file, `list --all`, `vm info` or `--verbose`; the real create, destroy and list
  output; volume names with their extensions; no bridge check; the cloud-init seed is plain ISO-9660, its
  instance-id is the VM name, and SSH keys go to the image's default user, not root; CLOUDINIT.md's validation
  list and example spec match the loader.

[Unreleased]: https://github.com/jbweber/foundry/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/jbweber/foundry/compare/v0.6.1...v0.7.0
