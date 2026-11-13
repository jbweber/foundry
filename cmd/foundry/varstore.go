package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
	"github.com/jbweber/foundry/internal/storage"
)

var (
	// varstoreArch is the guest architecture of 'varstore import' and 'delete'.
	varstoreArch string
	// varstoreReplace lets 'varstore import' replace an existing varstore.
	varstoreReplace bool
)

var varstoreCmd = &cobra.Command{
	Use:   "varstore",
	Short: "Manage Secure Boot key profiles (UEFI varstores)",
	Long: `Manage Secure Boot key profiles: UEFI varstores with a set of PK, KEK,
db and dbx entries enrolled, kept as volumes of the foundry-images pool.

A VM names a profile with spec.firmware.keys; on create, foundry looks up the
profile's varstore for the VM's architecture (spec.arch) and uses it as the
template of the VM's own varstore. Build the varstore with a standard tool,
e.g. virt-fw-vars from the virt-firmware project
(https://gitlab.com/kraxel/virt-firmware), enrolling into the stock OVMF
(x86_64) or AAVMF (aarch64) template.`,
}

func init() {
	varstoreCmd.AddCommand(varstoreImportCmd)
	varstoreCmd.AddCommand(varstoreListCmd)
	varstoreCmd.AddCommand(varstoreDeleteCmd)
	for _, c := range []*cobra.Command{varstoreImportCmd, varstoreDeleteCmd} {
		c.Flags().StringVar(&varstoreArch, "arch", v1alpha1.ArchX86_64,
			"guest architecture of the varstore (x86_64 or aarch64), as in spec.arch")
	}
	varstoreImportCmd.Flags().BoolVar(&varstoreReplace, "replace", false, "replace an existing varstore of this profile and architecture")
}

// checkVarstoreArgs validates a profile name and an architecture.
func checkVarstoreArgs(profile, arch string) error {
	if !loader.ValidKeyProfileName(profile) {
		return fmt.Errorf("profile name %q must be at most 63 lowercase letters, digits and dashes, starting and ending with a letter or digit", profile)
	}
	if arch != v1alpha1.ArchX86_64 && arch != v1alpha1.ArchAarch64 {
		return fmt.Errorf("--arch must be %s or %s, not %q", v1alpha1.ArchX86_64, v1alpha1.ArchAarch64, arch)
	}
	return nil
}

// withStorage runs fn with a storage manager whose default pools exist.
func withStorage(fn func(ctx context.Context, mgr *storage.Manager) error) error {
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
	if err := mgr.EnsureDefaultPools(ctx); err != nil {
		return fmt.Errorf("failed to ensure default pools: %w", err)
	}
	return fn(ctx, mgr)
}

// readVarstoreSource reads and validates a varstore from a file, or from in
// when source is "-".
func readVarstoreSource(source string, in io.Reader) ([]byte, error) {
	if source != "-" {
		f, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("failed to open varstore: %w", err)
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	return storage.ReadVarstore(in)
}

var varstoreImportCmd = &cobra.Command{
	Use:   "import <file|-> <profile>",
	Short: "Import a key profile's varstore into the foundry-images pool",
	Long: `Import a raw edk2 varstore (e.g. one made with virt-fw-vars) as the
varstore of a Secure Boot key profile for one architecture. "-" reads it from
standard input.

The file must be an edk2 variable store firmware volume ("_FVH" signature,
variable store GUID), a whole number of 4 KiB blocks and at most 64 MiB. It is
uploaded through libvirt as a raw volume of exactly its size, named
varstore_<profile>_<arch>.fd.

Replacing a profile (--replace) does not change existing VMs: libvirt copied
the template into each VM's own varstore on its first start.

Examples:
  foundry varstore import my-keys.fd my-keys
  foundry varstore import - my-keys --arch aarch64 < my-keys-aarch64.fd`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		source, profile := args[0], args[1]
		if err := checkVarstoreArgs(profile, varstoreArch); err != nil {
			return err
		}
		data, err := readVarstoreSource(source, cmd.InOrStdin())
		if err != nil {
			return err
		}
		return withStorage(func(ctx context.Context, mgr *storage.Manager) error {
			if err := mgr.ImportVarstore(ctx, profile, varstoreArch, data, varstoreReplace); err != nil {
				return err
			}
			fmt.Printf("✓ Key profile %s (%s) imported: %d bytes\n", profile, varstoreArch, len(data))
			return nil
		})
	},
}

var varstoreListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List the imported key profiles",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withStorage(func(ctx context.Context, mgr *storage.Manager) error {
			stores, err := mgr.ListVarstores(ctx)
			if err != nil {
				return err
			}
			if len(stores) == 0 {
				fmt.Println("No key profiles in the foundry-images pool")
				return nil
			}
			fmt.Printf("%-24s %-8s %10s  %s\n", "PROFILE", "ARCH", "BYTES", "PATH")
			for _, s := range stores {
				fmt.Printf("%-24s %-8s %10d  %s\n", s.Profile, s.Arch, s.Bytes, s.Path)
			}
			return nil
		})
	},
}

var varstoreDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a key profile's varstore",
	Long: `Delete a key profile's varstore for one architecture. Existing VMs keep
their own varstores; new VMs that name the profile fail to create until it is
imported again.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		profile := args[0]
		if err := checkVarstoreArgs(profile, varstoreArch); err != nil {
			return err
		}
		return withStorage(func(ctx context.Context, mgr *storage.Manager) error {
			if err := mgr.DeleteVarstore(ctx, profile, varstoreArch); err != nil {
				return err
			}
			fmt.Printf("✓ Key profile %s (%s) deleted\n", profile, varstoreArch)
			return nil
		})
	},
}
