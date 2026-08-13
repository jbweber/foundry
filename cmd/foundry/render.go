package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
)

var renderCmd = &cobra.Command{
	Use:   "render <config.yaml>",
	Short: "Render the libvirt domain XML for a config file",
	Long: `Render the libvirt domain XML that 'create' would define, without
connecting to libvirt or creating anything.

The config is loaded, defaulted, and validated exactly as 'create' does, so the
output matches what would be handed to libvirt. Useful for inspecting generated
XML — boot order, VLAN tags, disk targets — while iterating on a config.

Note that volume paths are rendered from the spec; the referenced pools, volumes,
and base images are not checked for existence.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		xml, err := renderDomainXML(args[0])
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), xml)
		return err
	},
}

// renderDomainXML loads a VM config and returns the domain XML that would be
// defined in libvirt for it.
func renderDomainXML(configPath string) (string, error) {
	vm, err := loader.LoadFromFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load configuration: %w", err)
	}

	xml, err := libvirt.GenerateDomainXML(vm)
	if err != nil {
		return "", fmt.Errorf("failed to generate domain XML: %w", err)
	}

	return xml, nil
}
