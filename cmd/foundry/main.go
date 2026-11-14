package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/output"
	"github.com/jbweber/foundry/internal/storage"
	"github.com/jbweber/foundry/internal/vm"
)

var (
	version = "dev"
	commit  = "unknown"

	// Global flags for output formatting
	outputFormat string
	noHeaders    bool

	// socketPath is --socket: the libvirt socket every command connects to
	socketPath string
)

func main() {
	ctx, stop := interruptContext()
	err := rootCmd.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// interruptContext returns a context that ends on the first SIGINT (Ctrl-C)
// or SIGTERM, so a command stops and cleans up what it made: create removes
// the volumes and domain it made so far, destroy stops before it undefines.
// Once it has ended, the signals are back to their default: a second Ctrl-C
// ends foundry at once, should the cleanup hang (what is left then is
// removed by 'foundry destroy').
func interruptContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			signal.Stop(sigs)
			fmt.Fprintln(os.Stderr, "Interrupted; cleaning up (press Ctrl-C again to quit at once)...")
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(sigs)
		cancel()
	}
}

var rootCmd = &cobra.Command{
	Use:   "foundry",
	Short: "Foundry - Libvirt VM management tool",
	Long: `Foundry is a CLI tool for managing libvirt VMs with simple YAML configuration.

It provides commands to create, destroy, and list virtual machines using
declarative configuration files.`,
	Version: fmt.Sprintf("%s (commit: %s)", version, commit),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		libvirt.SetSocket(socketPath)
	},
}

func init() {
	// Global persistent flags for output formatting
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|yaml|json)")
	rootCmd.PersistentFlags().BoolVar(&noHeaders, "no-headers", false, "Omit headers in table output")
	rootCmd.PersistentFlags().StringVar(&socketPath, "socket", "",
		"libvirt socket to connect to (default: virtqemud's socket if it exists, else libvirt-sock); "+
			"for a libvirt on another machine, a local socket forwarded over SSH")

	destroyCmd.Flags().BoolVar(&destroyForce, "force", false,
		"destroy a domain foundry did not make (no foundry spec in its metadata, or one that cannot be read)")

	// Subcommands will be added here
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(renderCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(testConnCmd)
	rootCmd.AddCommand(imageCmd)
	rootCmd.AddCommand(poolCmd)
	rootCmd.AddCommand(storageCmd)
	rootCmd.AddCommand(varstoreCmd)
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

		if err := vm.Create(cmd.Context(), configPath); err != nil {
			return fmt.Errorf("failed to create VM: %w", err)
		}

		fmt.Println("✓ VM created successfully!")
		return nil
	},
}

// destroyForce is 'destroy --force'.
var destroyForce bool

var destroyCmd = &cobra.Command{
	Use:   "destroy <vm-name>",
	Short: "Destroy a VM",
	Long: `Destroy a virtual machine by name.

This will:
- Gracefully shutdown the VM if running (5s timeout)
- Force destroy if still running
- Undefine the domain (with NVRAM cleanup)
- Delete the VM's storage volumes: those named <vm>_... in foundry-vms and,
  when its spec puts its disks in another pool (spec.storagePool), the boot,
  cloud-init, data and input volumes the spec names there. Images are never
  deleted by name prefix.

Only VMs foundry made are destroyed: a domain without foundry's spec in its
metadata, or whose spec cannot be read, is refused with nothing changed.
--force destroys it anyway (its <vm>_* volumes in foundry-vms only).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vmName := args[0]
		fmt.Printf("Destroying VM: %s\n", vmName)

		if err := vm.Destroy(cmd.Context(), vmName, destroyForce); err != nil {
			return fmt.Errorf("failed to destroy VM: %w", err)
		}

		fmt.Println("✓ VM destroyed successfully!")
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs",
	Long: `List all virtual machines (both running and stopped).

Shows VM name, phase, IP address, CPUs, memory, and age.

Output formats:
  -o table  Human-readable table (default)
  -o yaml   Full YAML resource definitions
  -o json   Full JSON resource definitions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate output format
		if err := output.ValidateFormat(outputFormat); err != nil {
			return err
		}

		ctx := context.Background()
		vms, err := vm.ListVMs(ctx)
		if err != nil {
			return fmt.Errorf("failed to list VMs: %w", err)
		}

		// Create formatter
		formatter, err := output.NewFormatter(output.Options{
			Format:    output.Format(outputFormat),
			NoHeaders: noHeaders,
		})
		if err != nil {
			return err
		}

		// Format and print
		result, err := formatter.FormatVMList(vms)
		if err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}

		fmt.Print(result)
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

		fmt.Printf("✓ Connected to libvirt daemon at %s\n", libvirt.Socket())

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
	Use:   "import <source-path|-> <name>",
	Short: "Import an image into the foundry-images pool",
	Long: `Import a base OS image from a local file into the foundry-images pool.
"-" reads the image from standard input.

The image file must be in QCOW2 or bootable RAW format. The image name must
include the correct file extension (.qcow2 or .raw) matching the actual format.

The tool validates that:
  - QCOW2 images have the correct magic bytes (QFI\xfb)
  - RAW images have a valid boot sector signature (0x55aa)
  - The file extension matches the detected format
  - The source is a regular file (or a symlink to one), or a pipe

This ensures only valid, bootable OS images are imported.

The file is streamed to libvirt, not read into memory. A raw image's volume
is exactly the file's size. If the import fails or is interrupted (Ctrl-C),
the partly written volume is deleted.

Standard input and pipes are first copied to a temporary file in $TMPDIR (else
/tmp), which needs room for the whole image: libvirt needs the volume's size
before the upload, and the checks read the image at several offsets. The copy
is removed when the import ends, however it ends.

Examples:
  # Import a QCOW2 image
  foundry image import /path/to/fedora-43.qcow2 fedora-43.qcow2

  # Import a bootable RAW image
  foundry image import /path/to/ubuntu-24.04.raw ubuntu-24.04.raw

  # Import from standard input
  xz -dc fedora-43.qcow2.xz | foundry image import - fedora-43.qcow2

  # This will fail - extension required
  foundry image import /path/to/fedora.qcow2 fedora

  # This will fail - format mismatch
  foundry image import /path/to/fedora.qcow2 fedora.raw`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]
		imageName := args[1]

		// A bad name fails before connecting or reading anything
		if err := storage.CheckImageName(imageName); err != nil {
			return err
		}

		from := sourcePath
		if from == "-" {
			from = "standard input"
		}
		fmt.Printf("Importing image from %s as %s...\n", from, imageName)

		in := cmd.InOrStdin()
		if sourcePath == "-" && isTerminal(in) {
			return fmt.Errorf("standard input is a terminal or another character device: pipe or redirect the image into it")
		}

		// Ctrl-C or SIGTERM cancels the upload, so the import deletes its
		// partial volume instead of leaving it under the image's name. Once
		// it has, the signals have their default effect again, so a second
		// Ctrl-C ends foundry if the cleanup hangs.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-ctx.Done()
			stop()
		}()

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
		if sourcePath == "-" {
			err = mgr.ImportImageFromReader(ctx, in, imageName)
		} else {
			err = mgr.ImportImage(ctx, sourcePath, imageName)
		}
		if err != nil {
			return fmt.Errorf("failed to import image: %w", err)
		}

		fmt.Printf("✓ Image %s imported successfully\n", imageName)
		return nil
	},
}

// isTerminal reports whether r is a terminal or another character device,
// where an image cannot come from: reading a terminal would wait for typing.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
