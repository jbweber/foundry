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
)

// Pool management commands
var poolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Manage storage pools",
	Long: `Manage libvirt storage pools for VM disks and images.

Storage pools are containers for storage volumes (disk images). Foundry uses
two default pools: foundry-images (base OS images) and foundry-vms (VM disks).`,
}

func init() {
	poolCmd.AddCommand(poolListCmd)
	poolCmd.AddCommand(poolInfoCmd)
	poolCmd.AddCommand(poolRefreshCmd)
	poolCmd.AddCommand(poolAddCmd)
	poolCmd.AddCommand(poolDeleteCmd)
}

var poolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all storage pools",
	Long: `List all storage pools with their state and capacity information.

Shows pool name, type, state, and storage capacity/usage for each pool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		mgr := storage.NewManager(client.Libvirt())
		pools, err := mgr.ListPools(ctx)
		if err != nil {
			return fmt.Errorf("failed to list pools: %w", err)
		}

		if len(pools) == 0 {
			fmt.Println("No storage pools found")
			return nil
		}

		// Print table header
		fmt.Printf("%-20s %-10s %-10s %12s %12s %12s\n",
			"NAME", "TYPE", "STATE", "CAPACITY", "ALLOCATED", "AVAILABLE")
		fmt.Println(strings.Repeat("-", 88))

		// Print each pool
		for _, pool := range pools {
			// Mark default pools
			name := pool.Name
			if pool.Name == storage.DefaultImagesPool || pool.Name == storage.DefaultVMsPool {
				name = pool.Name + " *"
			}

			fmt.Printf("%-20s %-10s %-10s %10.1fGB %10.1fGB %10.1fGB\n",
				name,
				pool.Type,
				pool.State,
				pool.CapacityGB(),
				pool.AllocationGB(),
				pool.AvailableGB(),
			)
		}

		fmt.Printf("\nTotal: %d pool(s)\n", len(pools))
		fmt.Println("* Default pools")
		return nil
	},
}

var poolInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about a pool",
	Long: `Display detailed information about a storage pool.

Shows pool name, type, path, state, UUID, and capacity/allocation details.

Example:
  foundry pool info foundry-images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolName := args[0]

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

		mgr := storage.NewManager(client.Libvirt())
		poolInfo, err := mgr.GetPoolInfo(ctx, poolName)
		if err != nil {
			return fmt.Errorf("failed to get pool info: %w", err)
		}

		// Get volume count
		volumes, err := mgr.ListVolumes(ctx, poolName)
		if err != nil {
			return fmt.Errorf("failed to list volumes: %w", err)
		}

		// Print pool details
		fmt.Printf("Pool: %s\n", poolInfo.Name)
		fmt.Printf("Type: %s\n", poolInfo.Type)
		fmt.Printf("State: %s\n", poolInfo.State)
		if poolInfo.Path != "" {
			fmt.Printf("Path: %s\n", poolInfo.Path)
		}
		fmt.Printf("UUID: %s\n", poolInfo.UUID)
		fmt.Printf("Capacity: %.2f GB (%d bytes)\n", poolInfo.CapacityGB(), poolInfo.Capacity)
		fmt.Printf("Allocated: %.2f GB (%d bytes)\n", poolInfo.AllocationGB(), poolInfo.Allocation)
		fmt.Printf("Available: %.2f GB (%d bytes)\n", poolInfo.AvailableGB(), poolInfo.Available)

		// Calculate usage percentage
		usagePercent := 0.0
		if poolInfo.Capacity > 0 {
			usagePercent = (float64(poolInfo.Allocation) / float64(poolInfo.Capacity)) * 100
		}
		fmt.Printf("Usage: %.1f%%\n", usagePercent)
		fmt.Printf("Volumes: %d\n", len(volumes))

		return nil
	},
}

var poolRefreshCmd = &cobra.Command{
	Use:   "refresh <name>",
	Short: "Refresh a storage pool",
	Long: `Refresh a storage pool to detect external changes.

This scans the pool's storage backend to update the list of volumes
and capacity information. Useful after manually adding/removing files.

Example:
  foundry pool refresh foundry-images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolName := args[0]

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

		mgr := storage.NewManager(client.Libvirt())
		if err := mgr.RefreshPool(ctx, poolName); err != nil {
			return fmt.Errorf("failed to refresh pool: %w", err)
		}

		fmt.Printf("✓ Pool %s refreshed successfully\n", poolName)
		return nil
	},
}

var poolAddCmd = &cobra.Command{
	Use:   "add <name> <type> <path>",
	Short: "Create a new storage pool",
	Long: `Create a new storage pool with the specified name, type, and path.

Currently only 'dir' (directory-based) pools are supported.

The pool will be:
  - Created and started immediately
  - Set to autostart on boot
  - Owned by the qemu user (typically uid/gid 107)

Example:
  foundry pool add my-pool dir /var/lib/libvirt/images/my-pool`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolName := args[0]
		poolTypeStr := args[1]
		poolPath := args[2]

		// Validate pool type
		poolType := storage.PoolType(poolTypeStr)
		if poolType != storage.PoolTypeDir {
			return fmt.Errorf("unsupported pool type: %s (only 'dir' is currently supported)", poolTypeStr)
		}

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

		mgr := storage.NewManager(client.Libvirt())

		fmt.Printf("Creating pool %s (type: %s, path: %s)...\n", poolName, poolType, poolPath)

		if err := mgr.CreatePool(ctx, poolName, poolType, poolPath); err != nil {
			return fmt.Errorf("failed to create pool: %w", err)
		}

		fmt.Printf("✓ Pool %s created successfully\n", poolName)
		return nil
	},
}

var poolDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a storage pool",
	Long: `Delete a storage pool by name.

Cannot delete default pools (foundry-images, foundry-vms).

Use --force to delete pools that contain volumes. Without --force,
only empty pools can be deleted.

Warning: Deleting a pool with --force will permanently delete all
volumes in the pool!

Example:
  foundry pool delete my-pool
  foundry pool delete my-pool --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		poolName := args[0]
		force, _ := cmd.Flags().GetBool("force")

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

		mgr := storage.NewManager(client.Libvirt())

		// Check if pool has volumes (for warning)
		volumes, err := mgr.ListVolumes(ctx, poolName)
		if err != nil {
			return fmt.Errorf("failed to check pool volumes: %w", err)
		}

		if len(volumes) > 0 {
			if !force {
				return fmt.Errorf("pool %s contains %d volume(s). Use --force to delete", poolName, len(volumes))
			}
			fmt.Printf("Warning: Deleting pool %s with %d volume(s)...\n", poolName, len(volumes))
		} else {
			fmt.Printf("Deleting pool %s...\n", poolName)
		}

		if err := mgr.DeletePool(ctx, poolName, force); err != nil {
			return fmt.Errorf("failed to delete pool: %w", err)
		}

		fmt.Printf("✓ Pool %s deleted successfully\n", poolName)
		return nil
	},
}

func init() {
	poolDeleteCmd.Flags().Bool("force", false, "Force deletion of pool with volumes")
}
