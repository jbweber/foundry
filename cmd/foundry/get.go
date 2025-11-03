package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/internal/output"
	"github.com/jbweber/foundry/internal/vm"
)

var getCmd = &cobra.Command{
	Use:   "get <vm-name>",
	Short: "Get details about a VM",
	Long: `Get detailed information about a specific virtual machine.

Displays the full VirtualMachine resource including spec and status.

Output formats:
  -o table  Human-readable table (default)
  -o yaml   Full YAML resource definition
  -o json   Full JSON resource definition`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vmName := args[0]

		// Validate output format
		if err := output.ValidateFormat(outputFormat); err != nil {
			return err
		}

		ctx := context.Background()
		vmObj, err := vm.GetVM(ctx, vmName)
		if err != nil {
			return fmt.Errorf("failed to get VM: %w", err)
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
		result, err := formatter.FormatVM(vmObj)
		if err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}

		fmt.Print(result)
		return nil
	},
}
