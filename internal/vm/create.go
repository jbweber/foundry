// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/cloudinit"
	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
	"github.com/jbweber/foundry/internal/metadata"
	"github.com/jbweber/foundry/internal/naming"
	"github.com/jbweber/foundry/internal/storage"
)

// Helper functions for volume naming
func getStoragePool(vm *v1alpha1.VirtualMachine) string {
	if vm.Spec.StoragePool == "" {
		return "foundry-vms"
	}
	return vm.Spec.StoragePool
}

func getBootVolumeName(vm *v1alpha1.VirtualMachine) string {
	return naming.VolumeNameBoot(vm.Name)
}

func getDataVolumeName(vm *v1alpha1.VirtualMachine, device string) string {
	return naming.VolumeNameData(vm.Name, device)
}

func getCloudInitVolumeName(vm *v1alpha1.VirtualMachine) string {
	return naming.VolumeNameCloudInit(vm.Name)
}

// parseImageReference parses an image reference and returns the pool and volume names.
// Supports three formats:
//   - Volume name only: "fedora-43.qcow2" -> uses ImagePool (or "foundry-images" default)
//   - Pool:volume format: "foundry-images:fedora-43.qcow2" -> explicit pool and volume
//   - File path: "/var/lib/libvirt/images/fedora.qcow2" -> returns empty strings (backward compat)
//
// Returns: (poolName, volumeName, isFilePath, error)
func parseImageReference(bootDisk v1alpha1.BootDiskSpec) (string, string, bool, error) {
	if bootDisk.Image == "" {
		return "", "", false, nil
	}

	// Check if it's a file path (contains / or starts with .)
	if strings.Contains(bootDisk.Image, "/") || strings.HasPrefix(bootDisk.Image, ".") {
		return "", "", true, nil
	}

	// Check for pool:volume format
	if strings.Contains(bootDisk.Image, ":") {
		parts := strings.SplitN(bootDisk.Image, ":", 2)
		if len(parts) != 2 {
			return "", "", false, fmt.Errorf("invalid pool:volume format: %q", bootDisk.Image)
		}
		poolName := strings.TrimSpace(parts[0])
		volumeName := strings.TrimSpace(parts[1])
		if poolName == "" || volumeName == "" {
			return "", "", false, fmt.Errorf("invalid pool:volume format: pool and volume cannot be empty")
		}
		return poolName, volumeName, false, nil
	}

	// Just a volume name - use ImagePool (or default)
	imagePool := bootDisk.ImagePool
	if imagePool == "" {
		imagePool = "foundry-images"
	}
	return imagePool, bootDisk.Image, false, nil
}

// verifyImageDigest reads back the image volume a VM's boot disk is backed
// by, when its spec states spec.bootDisk.imageSHA256, and refuses one that
// differs (storage.ErrImageDigest). It reads the whole volume, so it takes
// time in proportion to the image's size. The volume is the one create
// backs the boot disk with: imageName in foundry-images (the loader refuses
// a digest for an image in another pool).
func verifyImageDigest(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager, imageName string) error {
	want := vm.Spec.BootDisk.ImageSHA256
	if want == "" {
		return nil
	}
	log.Printf("Checking the sha256 of image %s:%s (reads the whole image)...", storage.DefaultImagesPool, imageName)
	got, err := sm.VolumeSHA256(ctx, storage.DefaultImagesPool, imageName)
	if err != nil {
		return fmt.Errorf("failed to check the digest of image %s: %w", imageName, err)
	}
	if got != want {
		return fmt.Errorf("%w: spec.bootDisk.imageSHA256 says image %s:%s has sha256 %s, it has sha256 %s (import it again, or correct the spec)",
			storage.ErrImageDigest, storage.DefaultImagesPool, imageName, want, got)
	}
	return nil
}

// Create creates a VM from a YAML configuration file.
//
// This orchestrates the entire VM creation process:
//  1. Load and validate configuration
//  2. Connect to libvirt
//  3. Pre-flight checks (VM exists, disk space, etc.)
//  4. Create storage (disks, cloud-init ISO, input disks)
//  5. Define domain in libvirt
//  6. Set autostart and start VM
//
// On any failure, attempts to clean up partially created resources.
//
// Returns an error if any step fails.
func Create(ctx context.Context, configPath string) error {
	// Load and validate configuration
	vm, err := loader.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return CreateFromConfig(ctx, vm)
}

// CreateFromConfig creates a VM from an already-loaded configuration.
//
// This is useful for testing and for callers that already have a config object.
// See Create() for the full workflow description.
func CreateFromConfig(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	// Connect to libvirt
	log.Printf("Connecting to libvirt...")
	LibvirtClient, err := foundrylibvirt.ConnectWithContext(ctx, "", 0)
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() {
		if err := LibvirtClient.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()

	// Create storage manager
	log.Printf("Initializing storage manager...")
	storageMgr := storage.NewManager(LibvirtClient.Libvirt())

	// Create metadata client
	metaClient := metadata.NewClient(LibvirtClient.Libvirt())

	// Ensure default pools exist
	log.Printf("Ensuring default storage pools exist...")
	if err := storageMgr.EnsureDefaultPools(ctx); err != nil {
		return fmt.Errorf("failed to ensure default pools: %w", err)
	}

	// Delegate to internal function with dependencies
	return createFromConfigWithDeps(ctx, vm, LibvirtClient.Libvirt(), storageMgr, metaClient)
}

// createFromConfigWithDeps creates a VM with injected dependencies.
// This allows for testing by accepting interfaces instead of concrete types.
func createFromConfigWithDeps(ctx context.Context, vm *v1alpha1.VirtualMachine, lv LibvirtClient, sm storageManager, mc *metadata.Client) error {
	// State tracking for cleanup
	var (
		domainDefined  bool
		storageCreated bool
	)

	// Setup cleanup function that runs on error
	var createErr error
	defer func() {
		if createErr != nil {
			cleanupWithDeps(ctx, vm, sm, lv, domainDefined, storageCreated)
		}
	}()

	// Step 1: Check if VM already exists
	log.Printf("Checking if VM '%s' already exists...", vm.Name)
	_, err := lv.DomainLookupByName(vm.Name)
	if err == nil {
		createErr = fmt.Errorf("VM '%s' already exists", vm.Name)
		return createErr
	}
	// Note: DomainLookupByName returns error if not found (which is what we want)

	// Step 2: Check if boot volume already exists (pre-flight check)
	log.Printf("Checking if boot volume already exists...")
	exists, createErr := sm.VolumeExists(ctx, getStoragePool(vm), getBootVolumeName(vm))
	if createErr != nil {
		return fmt.Errorf("failed to check boot volume: %w", createErr)
	}
	if exists {
		createErr = fmt.Errorf("boot volume already exists: %s/%s", getStoragePool(vm), getBootVolumeName(vm))
		return createErr
	}

	// Step 2b: Find the Secure Boot key profile's varstore, before any
	// storage is created, so a missing profile fails without cleanup
	var domainVM *v1alpha1.VirtualMachine
	domainVM, createErr = resolveKeyProfile(ctx, vm, sm)
	if createErr != nil {
		return createErr
	}

	// Step 2c: Read the input disks' files and build their images, and check
	// that none of their volumes exists, also before any storage is created
	var inputImages []inputImage
	inputImages, createErr = buildInputImages(vm)
	if createErr != nil {
		return createErr
	}
	if createErr = checkInputVolumes(ctx, vm, sm, inputImages); createErr != nil {
		return createErr
	}

	// Step 3: Parse image reference and get backing image path (if specified)
	var backingVolume string
	if vm.Spec.BootDisk.Image != "" && !vm.Spec.BootDisk.Empty {
		imagePool, imageName, isFilePath, parseErr := parseImageReference(vm.Spec.BootDisk)
		if parseErr != nil {
			createErr = parseErr
			return fmt.Errorf("failed to parse image reference: %w", createErr)
		}

		if isFilePath {
			// File path - use as-is for backward compatibility
			backingVolume = vm.Spec.BootDisk.Image
			log.Printf("Using backing image (file): %s", backingVolume)
		} else {
			// Pool-based image - verify it exists and get path
			log.Printf("Checking if backing image exists: %s:%s", imagePool, imageName)
			imageExists, checkErr := sm.ImageExists(ctx, imageName)
			if checkErr != nil {
				createErr = checkErr
				return fmt.Errorf("failed to check if image exists: %w", createErr)
			}
			if !imageExists {
				createErr = fmt.Errorf("backing image not found: %s (pool: %s). Import it with 'foundry image import'", imageName, imagePool)
				return createErr
			}

			// Get the filesystem path to the image volume
			backingVolume, createErr = sm.GetImagePath(ctx, imageName)
			if createErr != nil {
				return fmt.Errorf("failed to get image path: %w", createErr)
			}
			log.Printf("Using backing image (volume): %s", backingVolume)

			// The image's digest, when the spec states one, before anything
			// is made: a volume that is not the image it is named for (cut
			// short, or replaced) is never booted
			if createErr = verifyImageDigest(ctx, vm, sm, imageName); createErr != nil {
				return createErr
			}
		}
	}

	// Step 4: Create boot disk volume
	log.Printf("Creating boot disk volume (%dGB)...", vm.Spec.BootDisk.SizeGB)
	bootSpec := storage.VolumeSpec{
		Name:          getBootVolumeName(vm),
		Type:          storage.VolumeTypeBoot,
		Format:        storage.VolumeFormatQCOW2,
		CapacityGB:    uint64(vm.Spec.BootDisk.SizeGB),
		BackingVolume: backingVolume,
	}
	if createErr = sm.CreateVolume(ctx, getStoragePool(vm), bootSpec); createErr != nil {
		return fmt.Errorf("failed to create boot volume: %w", createErr)
	}
	storageCreated = true

	// Step 5: Create data disk volumes
	for _, dataDisk := range vm.Spec.DataDisks {
		log.Printf("Creating data disk volume %s (%dGB)...", dataDisk.Device, dataDisk.SizeGB)
		dataSpec := storage.VolumeSpec{
			Name:       getDataVolumeName(vm, dataDisk.Device),
			Type:       storage.VolumeTypeData,
			Format:     storage.VolumeFormatQCOW2,
			CapacityGB: uint64(dataDisk.SizeGB),
		}
		if createErr = sm.CreateVolume(ctx, getStoragePool(vm), dataSpec); createErr != nil {
			return fmt.Errorf("failed to create data volume %s: %w", dataDisk.Device, createErr)
		}
	}

	// Step 6: Generate and create cloud-init ISO volume (if configured)
	if vm.Spec.CloudInit != nil {
		log.Printf("Generating cloud-init ISO...")
		var isoData []byte
		isoData, createErr = cloudinit.GenerateISO(vm)
		if createErr != nil {
			return fmt.Errorf("failed to generate cloud-init ISO: %w", createErr)
		}

		log.Printf("Creating cloud-init ISO volume...")
		// Calculate ISO size in bytes and round up to nearest MB for capacity
		isoSizeBytes := uint64(len(isoData))
		isoSizeMB := (isoSizeBytes + 1024*1024 - 1) / (1024 * 1024) // Round up
		isoSizeGB := (isoSizeMB + 1024 - 1) / 1024                  // Round up to nearest GB
		if isoSizeGB == 0 {
			isoSizeGB = 1 // Minimum 1 GB for small ISOs
		}

		cloudInitSpec := storage.VolumeSpec{
			Name:       getCloudInitVolumeName(vm),
			Type:       storage.VolumeTypeCloudInit,
			Format:     storage.VolumeFormatRaw,
			CapacityGB: isoSizeGB,
		}
		if createErr = sm.CreateVolume(ctx, getStoragePool(vm), cloudInitSpec); createErr != nil {
			return fmt.Errorf("failed to create cloud-init volume: %w", createErr)
		}

		log.Printf("Writing cloud-init data to volume...")
		if createErr = sm.WriteVolumeData(ctx, getStoragePool(vm), getCloudInitVolumeName(vm), isoData); createErr != nil {
			return fmt.Errorf("failed to write cloud-init data: %w", createErr)
		}
	} else {
		log.Printf("Skipping cloud-init (not configured)")
	}

	// Step 7: Create the input disk volumes and upload their images
	if createErr = createInputVolumes(ctx, vm, sm, inputImages); createErr != nil {
		return createErr
	}

	// Step 9: Generate domain XML. A spec with spec.arch runs under KVM or
	// emulation depending on the hypervisor's architecture; one without is
	// x86_64 under KVM and needs no lookup.
	log.Printf("Generating domain XML...")
	var hostArch string
	if vm.Spec.Arch != "" {
		hostArch, createErr = foundrylibvirt.HostArch(lv)
		if createErr != nil {
			return fmt.Errorf("failed to determine hypervisor architecture: %w", createErr)
		}
		log.Printf("Guest %s on a %s hypervisor: domain type %s", vm.Spec.Arch, hostArch, vm.GetDomainType(hostArch))
	}
	var domainXML string
	domainXML, createErr = foundrylibvirt.GenerateDomainXML(domainVM, hostArch)
	if createErr != nil {
		return fmt.Errorf("failed to generate domain XML: %w", createErr)
	}

	// Step 10: Define domain in libvirt
	log.Printf("Defining domain in libvirt...")
	var domain libvirt.Domain
	domain, createErr = lv.DomainDefineXML(domainXML)
	if createErr != nil {
		return fmt.Errorf("failed to define domain: %w", createErr)
	}
	domainDefined = true

	// Step 11: Set autostart
	autostartValue := 1
	if vm.Spec.Autostart != nil && !*vm.Spec.Autostart {
		autostartValue = 0
	}
	log.Printf("Setting autostart to %d...", autostartValue)
	if createErr = lv.DomainSetAutostart(domain, int32(autostartValue)); createErr != nil {
		return fmt.Errorf("failed to set autostart: %w", createErr)
	}

	// Step 12: Start VM
	log.Printf("Starting VM...")
	if createErr = lv.DomainCreate(domain); createErr != nil {
		return fmt.Errorf("failed to start domain: %w", createErr)
	}

	// Step 13: Store VM metadata in libvirt domain
	log.Printf("Storing VM metadata...")
	if createErr = mc.Store(domain, vm); createErr != nil {
		log.Printf("Warning: failed to store VM metadata: %v", createErr)
		// Don't fail the creation if metadata storage fails - VM is already running
	}

	log.Printf("VM '%s' created successfully!", vm.Name)
	return nil
}

// resolveKeyProfile returns the VM to generate the domain XML from. Without
// spec.firmware.keys that is vm itself. With it, it is a copy whose
// nvramTemplate is the host path libvirt reports for the profile's varstore
// volume in the foundry-images pool: libvirt takes a varstore template only
// as a path, not as a pool volume, and copies it to the VM's own varstore on
// first start. vm itself, with the profile name, is what gets stored as the
// VM's spec.
func resolveKeyProfile(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager) (*v1alpha1.VirtualMachine, error) {
	fw := vm.Spec.Firmware
	if fw == nil || fw.Keys == "" {
		return vm, nil
	}
	arch := vm.GetArch()
	name := naming.VolumeNameVarstore(fw.Keys, arch)
	log.Printf("Looking up Secure Boot key profile %q (%s:%s)...", fw.Keys, storage.DefaultImagesPool, name)
	exists, err := sm.VolumeExists(ctx, storage.DefaultImagesPool, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check key profile varstore: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("key profile %q has no %s varstore in %s (import it with 'foundry varstore import <file> %s --arch %s')",
			fw.Keys, arch, storage.DefaultImagesPool, fw.Keys, arch)
	}
	path, err := sm.GetImagePath(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get key profile varstore path: %w", err)
	}
	log.Printf("Using varstore template (volume): %s", path)

	resolved := *vm
	resolvedFW := *fw
	resolvedFW.NVRAMTemplate = path
	resolvedFW.Keys = ""
	resolved.Spec.Firmware = &resolvedFW
	return &resolved, nil
}

// cleanupWithDeps attempts to clean up all VM resources on failure.
// This version accepts interfaces for testing.
//
// This is best-effort: it logs errors but continues trying to clean up
// as much as possible. It never returns an error.
func cleanupWithDeps(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager, lv LibvirtClient, domainDefined, storageCreated bool) {
	log.Printf("Cleaning up after failed VM creation...")

	// Clean up libvirt domain if it was defined
	if domainDefined && lv != nil {
		log.Printf("Undefining domain '%s'...", vm.Name)
		domain, err := lv.DomainLookupByName(vm.Name)
		if err != nil {
			log.Printf("Warning: failed to lookup domain for cleanup: %v", err)
		} else {
			// Try to destroy (force stop) if running
			if err := lv.DomainDestroy(domain); err != nil {
				// Ignore error - domain might not be running
				log.Printf("Note: domain was not running (this is normal): %v", err)
			}

			// Undefine the domain
			if err := lv.DomainUndefine(domain); err != nil {
				log.Printf("Warning: failed to undefine domain: %v", err)
			} else {
				log.Printf("Domain undefined successfully")
			}
		}
	}

	// Clean up storage volumes if any were created
	if storageCreated && sm != nil {
		log.Printf("Removing VM storage volumes...")

		// Delete boot volume
		if err := sm.DeleteVolume(ctx, getStoragePool(vm), getBootVolumeName(vm)); err != nil {
			log.Printf("Warning: failed to delete boot volume: %v", err)
		}

		// Delete data volumes
		for _, dataDisk := range vm.Spec.DataDisks {
			if err := sm.DeleteVolume(ctx, getStoragePool(vm), getDataVolumeName(vm, dataDisk.Device)); err != nil {
				log.Printf("Warning: failed to delete data volume %s: %v", dataDisk.Device, err)
			}
		}

		// Delete cloud-init ISO volume
		if vm.Spec.CloudInit != nil {
			if err := sm.DeleteVolume(ctx, getStoragePool(vm), getCloudInitVolumeName(vm)); err != nil {
				log.Printf("Warning: failed to delete cloud-init volume: %v", err)
			}
		}

		// Delete input disk volumes. create checked that none existed
		// before it, so these are its own.
		for _, in := range vm.Spec.InputDisks {
			if err := sm.DeleteVolume(ctx, getStoragePool(vm), foundrylibvirt.GetInputVolumeName(vm, in.Label)); err != nil {
				log.Printf("Warning: failed to delete input volume %s: %v", in.Label, err)
			}
		}

		log.Printf("Storage cleanup complete")
	}

	log.Printf("Cleanup complete")
}
