// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/plow/internal/cloudinit"
	"github.com/jbweber/plow/internal/config"
	"github.com/jbweber/plow/internal/disk"
	plowlibvirt "github.com/jbweber/plow/internal/libvirt"
)

// Create creates a VM from a YAML configuration file.
//
// This orchestrates the entire VM creation process:
//  1. Load and validate configuration
//  2. Connect to libvirt
//  3. Pre-flight checks (VM exists, disk space, etc.)
//  4. Create storage (directories, disks, cloud-init ISO)
//  5. Define domain in libvirt
//  6. Set autostart and start VM
//
// On any failure, attempts to clean up partially created resources.
//
// Returns an error if any step fails.
func Create(ctx context.Context, configPath string) error {
	// Load and validate configuration
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return CreateFromConfig(ctx, cfg)
}

// CreateFromConfig creates a VM from an already-loaded configuration.
//
// This is useful for testing and for callers that already have a config object.
// See Create() for the full workflow description.
func CreateFromConfig(ctx context.Context, cfg *config.VMConfig) error {
	// State tracking for cleanup
	var (
		libvirtClient  *plowlibvirt.Client
		storageManager *disk.Manager
		domainDefined  bool
		storageCreated bool
	)

	// Setup cleanup function that runs on error
	var createErr error
	defer func() {
		if createErr != nil {
			cleanup(ctx, cfg.Name, storageManager, libvirtClient, domainDefined, storageCreated)
		}
	}()

	// Step 1: Connect to libvirt
	log.Printf("Connecting to libvirt...")
	libvirtClient, createErr = plowlibvirt.ConnectWithContext(ctx, "", 0)
	if createErr != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", createErr)
	}
	defer func() {
		if err := libvirtClient.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()

	// Step 2: Check if VM already exists
	log.Printf("Checking if VM '%s' already exists...", cfg.Name)
	_, err := libvirtClient.Libvirt().DomainLookupByName(cfg.Name)
	if err == nil {
		createErr = fmt.Errorf("VM '%s' already exists", cfg.Name)
		return createErr
	}
	// Note: DomainLookupByName returns error if not found (which is what we want)

	// Step 3: Create storage manager
	log.Printf("Initializing storage manager...")
	storageManager, createErr = disk.NewManager()
	if createErr != nil {
		return fmt.Errorf("failed to create storage manager: %w", createErr)
	}

	// Step 4: Check disk space
	log.Printf("Checking disk space availability...")
	if createErr = storageManager.CheckDiskSpace(cfg); createErr != nil {
		return fmt.Errorf("disk space check failed: %w", createErr)
	}

	// Step 5: Check if VM directory already exists (should not)
	log.Printf("Checking if VM directory already exists...")
	exists, createErr := storageManager.VMDirectoryExists(cfg.Name)
	if createErr != nil {
		return fmt.Errorf("failed to check VM directory: %w", createErr)
	}
	if exists {
		createErr = fmt.Errorf("VM directory already exists: %s", storageManager.GetVMDirectory(cfg.Name))
		return createErr
	}

	// Step 6: Create VM directory
	log.Printf("Creating VM directory...")
	if createErr = storageManager.CreateVMDirectory(cfg.Name); createErr != nil {
		return fmt.Errorf("failed to create VM directory: %w", createErr)
	}
	storageCreated = true

	// Step 7: Create boot disk
	log.Printf("Creating boot disk (%dGB)...", cfg.BootDisk.SizeGB)
	if createErr = storageManager.CreateBootDisk(cfg); createErr != nil {
		return fmt.Errorf("failed to create boot disk: %w", createErr)
	}

	// Step 8: Create data disks
	for _, dataDisk := range cfg.DataDisks {
		log.Printf("Creating data disk %s (%dGB)...", dataDisk.Device, dataDisk.SizeGB)
		if createErr = storageManager.CreateDataDisk(cfg.Name, dataDisk); createErr != nil {
			return fmt.Errorf("failed to create data disk %s: %w", dataDisk.Device, createErr)
		}
	}

	// Step 9-10: Generate and write cloud-init ISO (if configured)
	if cfg.CloudInit != nil {
		log.Printf("Generating cloud-init ISO...")
		var isoData []byte
		isoData, createErr = cloudinit.GenerateISO(cfg)
		if createErr != nil {
			return fmt.Errorf("failed to generate cloud-init ISO: %w", createErr)
		}

		log.Printf("Writing cloud-init ISO...")
		if createErr = storageManager.WriteCloudInitISO(cfg, isoData); createErr != nil {
			return fmt.Errorf("failed to write cloud-init ISO: %w", createErr)
		}
	} else {
		log.Printf("Skipping cloud-init (not configured)")
	}

	// Step 11: Generate domain XML
	log.Printf("Generating domain XML...")
	var domainXML string
	domainXML, createErr = plowlibvirt.GenerateDomainXML(cfg)
	if createErr != nil {
		return fmt.Errorf("failed to generate domain XML: %w", createErr)
	}

	// Step 12: Define domain in libvirt
	log.Printf("Defining domain in libvirt...")
	var domain libvirt.Domain
	domain, createErr = libvirtClient.Libvirt().DomainDefineXML(domainXML)
	if createErr != nil {
		return fmt.Errorf("failed to define domain: %w", createErr)
	}
	domainDefined = true

	// Step 13: Set autostart
	log.Printf("Enabling autostart...")
	if createErr = libvirtClient.Libvirt().DomainSetAutostart(domain, 1); createErr != nil {
		return fmt.Errorf("failed to set autostart: %w", createErr)
	}

	// Step 14: Start VM
	log.Printf("Starting VM...")
	if createErr = libvirtClient.Libvirt().DomainCreate(domain); createErr != nil {
		return fmt.Errorf("failed to start domain: %w", createErr)
	}

	log.Printf("VM '%s' created successfully!", cfg.Name)
	return nil
}

// cleanup attempts to clean up all VM resources on failure.
//
// This is best-effort: it logs errors but continues trying to clean up
// as much as possible. It never returns an error.
//
// Parameters:
//   - ctx: Context for operations (may be cancelled)
//   - vmName: Name of the VM to clean up
//   - storageManager: Storage manager (may be nil if not yet created)
//   - libvirtClient: Libvirt client (may be nil if not yet connected)
//   - domainDefined: Whether the domain was successfully defined
//   - storageCreated: Whether any storage was created
func cleanup(ctx context.Context, vmName string, storageManager *disk.Manager,
	libvirtClient *plowlibvirt.Client, domainDefined, storageCreated bool) {

	log.Printf("Cleaning up after failed VM creation...")

	// Clean up libvirt domain if it was defined
	if domainDefined && libvirtClient != nil {
		log.Printf("Undefining domain '%s'...", vmName)
		domain, err := libvirtClient.Libvirt().DomainLookupByName(vmName)
		if err != nil {
			log.Printf("Warning: failed to lookup domain for cleanup: %v", err)
		} else {
			// Try to destroy (force stop) if running
			if err := libvirtClient.Libvirt().DomainDestroy(domain); err != nil {
				// Ignore error - domain might not be running
				log.Printf("Note: domain was not running (this is normal): %v", err)
			}

			// Undefine the domain
			if err := libvirtClient.Libvirt().DomainUndefine(domain); err != nil {
				log.Printf("Warning: failed to undefine domain: %v", err)
			} else {
				log.Printf("Domain undefined successfully")
			}
		}
	}

	// Clean up storage if any was created
	if storageCreated && storageManager != nil {
		log.Printf("Removing VM storage...")
		if err := storageManager.DeleteVM(vmName); err != nil {
			log.Printf("Warning: failed to delete VM storage: %v", err)
		} else {
			log.Printf("Storage removed successfully")
		}
	}

	log.Printf("Cleanup complete")
}
