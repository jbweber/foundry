package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
)

// renderHostArch is 'render --host-arch': the hypervisor architecture a spec
// with spec.arch is rendered for, which decides between KVM and emulation.
var renderHostArch string

func init() {
	renderCmd.Flags().StringVar(&renderHostArch, "host-arch", v1alpha1.ArchFromGOARCH(runtime.GOARCH),
		"hypervisor architecture to render for (x86_64 or aarch64)")
}

var renderCmd = &cobra.Command{
	Use:   "render <config.yaml>",
	Short: "Render the libvirt domain XML for a config file",
	Long: `Render the libvirt domain XML that 'create' would define, without
connecting to libvirt or creating anything.

The config is loaded, defaulted, and validated exactly as 'create' does, so the
output matches what would be handed to libvirt. Useful for inspecting generated
XML — boot order, VLAN tags, disk targets — while iterating on a config.

Note that volume paths are rendered from the spec; the referenced pools, volumes,
and base images are not checked for existence.

A spec with spec.arch runs under KVM when it matches the hypervisor's
architecture and under QEMU emulation otherwise. render does not ask libvirt,
so it assumes the machine it runs on, unless --host-arch names another. A spec
without spec.arch is an x86_64 guest under KVM either way.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		xml, err := renderDomainXML(args[0], renderHostArch)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), xml)
		return err
	},
}

// renderDomainXML loads a VM config and returns the domain XML that would be
// defined in libvirt for it on a hypervisor of architecture hostArch.
func renderDomainXML(configPath, hostArch string) (string, error) {
	switch hostArch {
	case v1alpha1.ArchX86_64, v1alpha1.ArchAarch64:
	default:
		return "", fmt.Errorf("--host-arch %q is not supported (expected %s or %s)", hostArch, v1alpha1.ArchX86_64, v1alpha1.ArchAarch64)
	}

	vm, err := loader.LoadFromFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load configuration: %w", err)
	}

	xml, err := libvirt.GenerateDomainXML(vm, hostArch)
	if err != nil {
		return "", fmt.Errorf("failed to generate domain XML: %w", err)
	}

	return xml, nil
}
