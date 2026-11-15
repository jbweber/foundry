package libvirt

import (
	"fmt"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// archSettings is what differs in a domain definition between guest
// architectures. Everything else (UEFI firmware, pcie-root, virtio disks,
// NICs, balloon, RNG, serial console) is common to all.
// Every architecture foundry supports has an entry in archs, and an architecture
// without one is refused, never given another's settings.
type archSettings struct {
	// machine is QEMU's machine type, the same for every guest of the
	// architecture, and the one domain capabilities are asked about.
	machine string
	// features returns the hypervisor features the guest gets.
	features func() *libvirtxml.DomainFeatureList
	// biosSerial emits <bios useserial='yes'/>.
	biosSerial bool
	// cpuModelFallback emits <model fallback='allow'/> in <cpu>.
	cpuModelFallback bool
	// timers returns the clock timers the guest gets (nil for none).
	timers func() []libvirtxml.DomainTimer
	// cdromBus is the bus of the CD-ROMs, and cdromController the controller
	// model it needs added ("" when the machine has one built in).
	cdromBus, cdromController string
}

// archs holds the settings of each guest architecture, by libvirt's name.
var archs = map[string]archSettings{
	// q35, QEMU's PCIe x86 machine.
	v1alpha1.ArchX86_64: {
		machine: "q35",
		features: func() *libvirtxml.DomainFeatureList {
			return &libvirtxml.DomainFeatureList{
				ACPI: &libvirtxml.DomainFeature{},
				APIC: &libvirtxml.DomainFeatureAPIC{},
				PAE:  &libvirtxml.DomainFeature{},
			}
		},
		// The Serial Graphics Adapter sends firmware output to the serial port.
		biosSerial:       true,
		cpuModelFallback: true,
		timers: func() []libvirtxml.DomainTimer {
			return []libvirtxml.DomainTimer{
				{Name: "rtc", TickPolicy: "catchup"},
				{Name: "pit", TickPolicy: "delay"},
				{Name: "hpet", Present: "no"},
			}
		},
		// q35 has an AHCI controller built in.
		cdromBus: "sata",
	},
	// virt, QEMU's generic Arm machine.
	v1alpha1.ArchAarch64: {
		machine: "virt",
		// Arm uses a GIC rather than an APIC, and has no PAE. GICv3 works under
		// both KVM on current Arm servers and TCG; "host" would fail under TCG
		// (https://libvirt.org/formatdomain.html#hypervisor-features).
		features: func() *libvirtxml.DomainFeatureList {
			return &libvirtxml.DomainFeatureList{
				ACPI: &libvirtxml.DomainFeature{},
				GIC:  &libvirtxml.DomainFeatureGIC{Version: "3"},
			}
		},
		// <bios useserial> drives the x86 Serial Graphics Adapter; libvirt
		// rejects it on other architectures ("BIOS serial console only supported
		// on x86 architectures", qemuValidateDomainDef in
		// https://gitlab.com/libvirt/libvirt/-/blob/master/src/qemu/qemu_validate.c).
		biosSerial: false,
		// The model fallback only matters for x86 named CPU models.
		cpuModelFallback: false,
		// rtc, pit and hpet are x86 timers; the Arm guest uses the architected
		// generic timer, which needs no configuration.
		timers: func() []libvirtxml.DomainTimer { return nil },
		// virt has no AHCI controller, so SATA is unavailable; virt-install also
		// gives every non-x86 guest that supports it SCSI CD-ROMs on virtio-scsi:
		// https://github.com/virt-manager/virt-manager/blob/main/virtinst/devices/disk.py (_default_bus)
		cdromBus:        "scsi",
		cdromController: "virtio-scsi",
	},
}

// settingsFor returns an architecture's settings, or an error naming it.
func settingsFor(arch string) (archSettings, error) {
	s, ok := archs[arch]
	if !ok {
		return archSettings{}, fmt.Errorf("architecture %q is not supported (expected %s or %s)", arch, v1alpha1.ArchX86_64, v1alpha1.ArchAarch64)
	}
	return s, nil
}

// MachineType returns the QEMU machine type foundry gives guests of an
// architecture (libvirt's name): q35 for x86_64, virt for aarch64.
func MachineType(arch string) (string, error) {
	s, err := settingsFor(arch)
	if err != nil {
		return "", err
	}
	return s.machine, nil
}
