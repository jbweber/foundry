package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/storage"
	"github.com/jbweber/foundry/internal/vm"
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
	Use:   "foundry",
	Short: "Foundry - Libvirt VM management tool",
	Long: `Foundry is a CLI tool for managing libvirt VMs with simple YAML configuration.

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
	rootCmd.AddCommand(imageCmd)
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

		ctx := context.Background()
		if err := vm.Create(ctx, configPath); err != nil {
			return fmt.Errorf("failed to create VM: %w", err)
		}

		fmt.Println("✓ VM created successfully!")
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

// Image management commands
var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage base images",
	Long: `Manage base OS images in the foundry-images storage pool.

Base images are used as backing files for VM boot disks, allowing
quick VM creation without duplicating disk space.`,
}

func init() {
	imageCmd.AddCommand(imageImportCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imageDeleteCmd)
	imageCmd.AddCommand(imageInfoCmd)
}

var imageImportCmd = &cobra.Command{
	Use:   "import <source-path> <name>",
	Short: "Import an image into the foundry-images pool",
	Long: `Import a base OS image from a local file into the foundry-images pool.

The image will be stored in the foundry-images pool and can be referenced
by name when creating VMs.

Example:
  foundry image import /path/to/fedora-43.qcow2 fedora-43`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]
		imageName := args[1]

		fmt.Printf("Importing image from %s as %s...\n", sourcePath, imageName)

		// Connect to libvirt
		ctx := context.Background()
		client, err := libvirt.Connect("", 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to libvirt: %w", err)
		}
		defer func() {
			if closeErr := client.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close libvirt connection: %v\n", closeErr)
			}
		}()

		// Create storage manager
		mgr := storage.NewManager(client.Libvirt())

		// Ensure default pools exist
		if err := mgr.EnsureDefaultPools(ctx); err != nil {
			return fmt.Errorf("failed to ensure default pools: %w", err)
		}

		// Check if image already exists
		exists, err := mgr.ImageExists(ctx, imageName)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}
		if exists {
			return fmt.Errorf("image %s already exists", imageName)
		}

		// Import the image
		if err := mgr.ImportImage(ctx, sourcePath, imageName); err != nil {
			return fmt.Errorf("failed to import image: %w", err)
		}

		fmt.Printf("✓ Image %s imported successfully\n", imageName)
		return nil
	},
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all images in the foundry-images pool",
	Long: `List all base OS images stored in the foundry-images pool.

Shows image name, format, size, and path for each image.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Connect to libvirt
		ctx := context.Background()
		client, err := libvirt.Connect("", 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to libvirt: %w", err)
		}
		defer func() {
			if closeErr := client.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close libvirt connection: %v\n", closeErr)
			}
		}()

		// Create storage manager
		mgr := storage.NewManager(client.Libvirt())

		// Ensure default pools exist
		if err := mgr.EnsureDefaultPools(ctx); err != nil {
			return fmt.Errorf("failed to ensure default pools: %w", err)
		}

		// List images
		images, err := mgr.ListImages(ctx)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}

		if len(images) == 0 {
			fmt.Println("No images found in foundry-images pool")
			return nil
		}

		// Print table header
		fmt.Printf("%-30s %-10s %10s  %s\n", "NAME", "FORMAT", "SIZE", "PATH")
		fmt.Println(strings.Repeat("-", 100))

		// Print each image
		for _, img := range images {
			fmt.Printf("%-30s %-10s %8.1fGB  %s\n",
				img.Name,
				img.Format,
				img.CapacityGB(),
				img.Path,
			)
		}

		fmt.Printf("\nTotal: %d image(s)\n", len(images))
		return nil
	},
}

var imageDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an image from the foundry-images pool",
	Long: `Delete a base OS image from the foundry-images pool.

Warning: This will permanently delete the image. VMs that use this image
as a backing file may become unusable.

Example:
  foundry image delete fedora-43`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageName := args[0]

		fmt.Printf("Deleting image %s...\n", imageName)

		// Connect to libvirt
		ctx := context.Background()
		client, err := libvirt.Connect("", 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to libvirt: %w", err)
		}
		defer func() {
			if closeErr := client.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close libvirt connection: %v\n", closeErr)
			}
		}()

		// Create storage manager
		mgr := storage.NewManager(client.Libvirt())

		// Ensure default pools exist
		if err := mgr.EnsureDefaultPools(ctx); err != nil {
			return fmt.Errorf("failed to ensure default pools: %w", err)
		}

		// Check if image exists
		exists, err := mgr.ImageExists(ctx, imageName)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}
		if !exists {
			return fmt.Errorf("image %s not found", imageName)
		}

		// Delete the image
		if err := mgr.DeleteImage(ctx, imageName, false); err != nil {
			return fmt.Errorf("failed to delete image: %w", err)
		}

		fmt.Printf("✓ Image %s deleted successfully\n", imageName)
		return nil
	},
}

var imageInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about an image",
	Long: `Display detailed information about a base OS image in the foundry-images pool.

Shows image name, format, capacity, allocation, path, and other metadata.

Example:
  foundry image info fedora-43`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageName := args[0]

		// Connect to libvirt
		ctx := context.Background()
		client, err := libvirt.Connect("", 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to libvirt: %w", err)
		}
		defer func() {
			if closeErr := client.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close libvirt connection: %v\n", closeErr)
			}
		}()

		// Create storage manager
		mgr := storage.NewManager(client.Libvirt())

		// Ensure default pools exist
		if err := mgr.EnsureDefaultPools(ctx); err != nil {
			return fmt.Errorf("failed to ensure default pools: %w", err)
		}

		// Check if image exists
		exists, err := mgr.ImageExists(ctx, imageName)
		if err != nil {
			return fmt.Errorf("failed to check if image exists: %w", err)
		}
		if !exists {
			return fmt.Errorf("image %s not found", imageName)
		}

		// Get image info (list all images and find the one we want)
		images, err := mgr.ListImages(ctx)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}

		var imageInfo *storage.VolumeInfo
		for _, img := range images {
			if img.Name == imageName {
				imageInfo = &img
				break
			}
		}

		if imageInfo == nil {
			return fmt.Errorf("image %s not found", imageName)
		}

		// Print image details
		fmt.Printf("Image: %s\n", imageInfo.Name)
		fmt.Printf("Pool: %s\n", imageInfo.Pool)
		fmt.Printf("Format: %s\n", imageInfo.Format)
		fmt.Printf("Type: %s\n", imageInfo.Type)
		fmt.Printf("Capacity: %.2f GB (%d bytes)\n", imageInfo.CapacityGB(), imageInfo.Capacity)
		fmt.Printf("Allocation: %.2f GB (%d bytes)\n", imageInfo.AllocationGB(), imageInfo.Allocation)
		fmt.Printf("Path: %s\n", imageInfo.Path)

		return nil
	},
}
