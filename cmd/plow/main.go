package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "plow",
	Short: "Plow - Libvirt VM management tool",
	Long: `Plow is a CLI tool for managing libvirt VMs with simple YAML configuration.

It provides commands to create, destroy, and list virtual machines using
declarative configuration files.`,
	Version: fmt.Sprintf("%s (commit: %s)", version, commit),
}

func init() {
	// Subcommands will be added here
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(listCmd)
}

var createCmd = &cobra.Command{
	Use:   "create <config.yaml>",
	Short: "Create a VM from a configuration file",
	Long: `Create a new virtual machine from a YAML configuration file.

The configuration file defines the VM's resources (CPU, memory, disk),
network settings, and cloud-init configuration.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := args[0]
		fmt.Printf("Creating VM from config: %s\n", configPath)
		fmt.Println("(Not yet implemented)")
		return nil
	},
}

var destroyCmd = &cobra.Command{
	Use:   "destroy <vm-name>",
	Short: "Destroy a VM",
	Long: `Destroy a virtual machine by name.

This will:
- Stop the VM if running
- Undefine the domain
- Delete all storage volumes
- Remove the VM directory`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vmName := args[0]
		fmt.Printf("Destroying VM: %s\n", vmName)
		fmt.Println("(Not yet implemented)")
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List VMs",
	Long: `List all virtual machines managed by libvirt.

Shows VM name, state, resources, and IP addresses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Listing VMs...")
		fmt.Println("(Not yet implemented)")
		return nil
	},
}
