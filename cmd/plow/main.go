package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbweber/plow/internal/libvirt"
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
	rootCmd.AddCommand(testConnCmd)
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

var testConnCmd = &cobra.Command{
	Use:   "test-conn",
	Short: "Test libvirt connection",
	Long:  `Test connectivity to the libvirt daemon and display version information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Testing libvirt connection...")

		// Connect to libvirt
		client, err := libvirt.Connect("", 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to libvirt: %w", err)
		}
		defer func() {
			if closeErr := client.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close libvirt connection: %v\n", closeErr)
			}
		}()

		fmt.Println("✓ Connected to libvirt daemon")

		// Ping the connection
		if err := client.Ping(); err != nil {
			return fmt.Errorf("connection test failed: %w", err)
		}

		// Get libvirt version
		version, err := client.Libvirt().ConnectGetLibVersion()
		if err != nil {
			return fmt.Errorf("failed to get libvirt version: %w", err)
		}

		// Format version (libvirt returns version as an integer like 8006000 for 8.6.0)
		major := version / 1000000
		minor := (version % 1000000) / 1000
		patch := version % 1000

		fmt.Printf("✓ Libvirt version: %d.%d.%d\n", major, minor, patch)

		// Get hostname
		hostname, err := client.Libvirt().ConnectGetHostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}

		fmt.Printf("✓ Hypervisor hostname: %s\n", hostname)

		// Get connection URI
		uri, err := client.Libvirt().ConnectGetUri()
		if err != nil {
			return fmt.Errorf("failed to get connection URI: %w", err)
		}

		fmt.Printf("✓ Connection URI: %s\n", uri)

		fmt.Println("\nConnection test successful!")
		return nil
	},
}
