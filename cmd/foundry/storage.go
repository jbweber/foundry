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

// Storage management commands
var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage storage",
	Long: `View and manage storage pools and volumes.

Provides overview of all storage pools and their usage.`,
}

func init() {
	storageCmd.AddCommand(storageStatusCmd)
}

var storageStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show storage status overview",
	Long: `Display an overview of all storage pools with capacity and usage information.

Shows a summary of total storage across all pools, followed by detailed
information for each pool including volume counts and usage percentage.`,
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

		// Calculate totals
		var totalCapacity, totalAllocation, totalAvailable uint64
		var totalVolumes int
		runningPools := 0
		inactivePools := 0

		for _, pool := range pools {
			totalCapacity += pool.Capacity
			totalAllocation += pool.Allocation
			totalAvailable += pool.Available

			if pool.State == "running" {
				runningPools++
			} else {
				inactivePools++
			}

			// Get volume count
			volumes, err := mgr.ListVolumes(ctx, pool.Name)
			if err == nil {
				totalVolumes += len(volumes)
			}
		}

		// Print summary
		fmt.Println("Storage Overview")
		fmt.Println(strings.Repeat("=", 88))
		fmt.Printf("Pools:      %d total (%d running, %d inactive)\n", len(pools), runningPools, inactivePools)
		fmt.Printf("Volumes:    %d total\n", totalVolumes)
		fmt.Printf("Capacity:   %.2f GB\n", float64(totalCapacity)/(1024*1024*1024))
		fmt.Printf("Allocated:  %.2f GB\n", float64(totalAllocation)/(1024*1024*1024))
		fmt.Printf("Available:  %.2f GB\n", float64(totalAvailable)/(1024*1024*1024))

		totalUsagePercent := 0.0
		if totalCapacity > 0 {
			totalUsagePercent = (float64(totalAllocation) / float64(totalCapacity)) * 100
		}
		fmt.Printf("Usage:      %.1f%%\n", totalUsagePercent)

		// Print detailed pool breakdown
		fmt.Println()
		fmt.Println("Pool Details")
		fmt.Println(strings.Repeat("=", 88))
		fmt.Printf("%-20s %-10s %8s %12s %12s %12s %8s\n",
			"NAME", "STATE", "VOLUMES", "CAPACITY", "ALLOCATED", "AVAILABLE", "USAGE")
		fmt.Println(strings.Repeat("-", 88))

		for _, pool := range pools {
			// Get volume count
			volumes, err := mgr.ListVolumes(ctx, pool.Name)
			volumeCount := 0
			if err == nil {
				volumeCount = len(volumes)
			}

			// Calculate usage percentage
			usagePercent := 0.0
			if pool.Capacity > 0 {
				usagePercent = (float64(pool.Allocation) / float64(pool.Capacity)) * 100
			}

			// Mark default pools
			name := pool.Name
			if pool.Name == storage.DefaultImagesPool || pool.Name == storage.DefaultVMsPool {
				name = pool.Name + " *"
			}

			// Format state with indicator
			stateIndicator := "○" // inactive
			if pool.State == "running" {
				stateIndicator = "●" // running
			}
			stateStr := fmt.Sprintf("%s %s", stateIndicator, pool.State)

			fmt.Printf("%-20s %-10s %8d %10.1fGB %10.1fGB %10.1fGB %7.1f%%\n",
				name,
				stateStr,
				volumeCount,
				pool.CapacityGB(),
				pool.AllocationGB(),
				pool.AvailableGB(),
				usagePercent,
			)
		}

		fmt.Println()
		fmt.Println("● running  ○ inactive  * default pool")
		return nil
	},
}
