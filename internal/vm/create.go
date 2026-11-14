// Package vm provides high-level VM management operations.
package vm

import (
	"context"
	"errors"
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
//  5. Define domain in libvirt and store the spec in it
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
func createFromConfigWithDeps(ctx context.Context, vm *v1alpha1.VirtualMachine, lv LibvirtClient, sm storageManager, mc *metadata.Client) (retErr error) {
	// State tracking for cleanup
	var (
		domainDefined bool
		// made lists the volumes this create made, in the VM's pool, as it
		// makes them: a failed create deletes these and nothing else
		made []string
	)

	// Setup cleanup function that runs on error. What the cleanup could not
	// remove is added to the create's error, so it is not left unnoticed
	var createErr error
	defer func() {
		if createErr != nil {
			if err := cleanupWithDeps(ctx, vm, sm, lv, domainDefined, made); err != nil {
				retErr = fmt.Errorf("%w; cleanup incomplete: %w", retErr, err)
			}
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

	// Step 1b: The spec must fit in the domain's metadata, where it is stored
	// after the define; checked now so a spec too large to store fails
	// before anything is made
	if _, createErr = metadata.Encode(vm); createErr != nil {
		return createErr
	}

	// Step 2: Check that none of the VM's boot, data and cloud-init volumes
	// exists yet (pre-flight check; input volumes are checked in step 2c)
	log.Printf("Checking that the VM's volumes do not exist yet...")
	if createErr = checkVolumesAbsent(ctx, vm, sm); createErr != nil {
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
	var backingFormat storage.VolumeFormat
	if vm.Spec.BootDisk.Image != "" && !vm.Spec.BootDisk.Empty {
		imagePool, imageName, isFilePath, parseErr := parseImageReference(vm.Spec.BootDisk)
		if parseErr != nil {
			createErr = parseErr
			return fmt.Errorf("failed to parse image reference: %w", createErr)
		}

		if isFilePath {
			// File path - checked as import checks an image, its format
			// taken from its content, before anything is made
			backingVolume = vm.Spec.BootDisk.Image
			backingFormat, createErr = storage.CheckBootImageFile(backingVolume)
			if createErr != nil {
				return createErr
			}
			log.Printf("Using backing image (file, %s): %s", backingFormat, backingVolume)
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
		BackingFormat: backingFormat,
	}
	if createErr = sm.CreateVolume(ctx, getStoragePool(vm), bootSpec); createErr != nil {
		return fmt.Errorf("failed to create boot volume: %w", createErr)
	}
	made = append(made, bootSpec.Name)

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
		made = append(made, dataSpec.Name)
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
		made = append(made, cloudInitSpec.Name)

		log.Printf("Writing cloud-init data to volume...")
		if createErr = sm.WriteVolumeData(ctx, getStoragePool(vm), getCloudInitVolumeName(vm), isoData); createErr != nil {
			return fmt.Errorf("failed to write cloud-init data: %w", createErr)
		}
	} else {
		log.Printf("Skipping cloud-init (not configured)")
	}

	// Step 7: Create the input disk volumes and upload their images
	if createErr = createInputVolumes(ctx, vm, sm, inputImages, &made); createErr != nil {
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

	// Step 10b: Store the VM's spec in the domain's persistent definition,
	// before the domain starts: the domain is foundry's from the moment it
	// exists, and a VM foundry cannot store the spec of is not created (it
	// would be one destroy refuses as not foundry's)
	log.Printf("Storing VM metadata...")
	if createErr = mc.Store(domain, vm); createErr != nil {
		return fmt.Errorf("failed to store VM metadata: %w", createErr)
	}

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

// cleanupWithDeps removes the VM resources a failed create made: the
// domain, if it was defined, and the volumes in made (names in the VM's
// pool). This version accepts interfaces for testing.
//
// It carries on past a failure where that is safe, and returns an error that
// says what was left behind. A domain that may still be running (it could
// not be stopped, or its state could not be read) is left defined with all
// its volumes: undefining an active domain only makes it transient, and its
// disks would be deleted under it. Such a VM's spec is stored, so
// `foundry destroy` removes it once it can be stopped.
func cleanupWithDeps(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager, lv LibvirtClient, domainDefined bool, made []string) error {
	log.Printf("Cleaning up after failed VM creation...")
	var left []string

	if domainDefined && lv != nil {
		if err := removeDomain(lv, vm.Name); err != nil {
			if errors.Is(err, errDomainMayRun) {
				for _, name := range made {
					left = append(left, fmt.Sprintf("volume %s/%s", getStoragePool(vm), name))
				}
				return fmt.Errorf("%w; left in place: %s (remove with 'foundry destroy %s')", err, strings.Join(left, ", "), vm.Name)
			}
			left = append(left, err.Error())
		}
	}

	// Delete the volumes this create made, and only those: a name the spec
	// gives may be a volume that existed before (create refuses one it
	// finds, but libvirt has the last word), and that is not ours to delete
	if len(made) > 0 && sm != nil {
		log.Printf("Removing VM storage volumes...")
		for _, name := range made {
			if err := sm.DeleteVolume(ctx, getStoragePool(vm), name); err != nil {
				log.Printf("Warning: failed to delete volume %s: %v", name, err)
				left = append(left, fmt.Sprintf("volume %s/%s not deleted: %v", getStoragePool(vm), name, err))
			}
		}
	}

	if len(left) > 0 {
		return errors.New(strings.Join(left, "; "))
	}
	log.Printf("Cleanup complete")
	return nil
}

// errDomainMayRun marks a cleanup failure after which the domain may still
// be running.
var errDomainMayRun = errors.New("may still be running")

// removeDomain stops (forcibly) and undefines the domain a failed create
// defined, with its varstore: every foundry domain is UEFI, and once a start
// has made the varstore libvirt refuses an undefine without
// VIR_DOMAIN_UNDEFINE_NVRAM ("cannot undefine domain with nvram"). A domain
// that is already gone is not an error.
func removeDomain(lv LibvirtClient, name string) error {
	log.Printf("Undefining domain '%s'...", name)
	domain, err := lv.DomainLookupByName(name)
	if err != nil {
		var le libvirt.Error
		if errors.As(err, &le) && le.Code == uint32(libvirt.ErrNoDomain) {
			return nil
		}
		return fmt.Errorf("domain '%s' %w: lookup failed: %w", name, errDomainMayRun, err)
	}
	state, _, err := lv.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("domain '%s' %w: its state cannot be read: %w", name, errDomainMayRun, err)
	}
	if state != domainStateShutoff {
		if err := lv.DomainDestroy(domain); err != nil {
			return fmt.Errorf("domain '%s' %w: it could not be stopped: %w", name, errDomainMayRun, err)
		}
	}
	if err := lv.DomainUndefineFlags(domain, libvirt.DomainUndefineNvram); err != nil {
		log.Printf("Warning: failed to undefine domain: %v", err)
		return fmt.Errorf("domain '%s' left defined (stopped): %w", name, err)
	}
	log.Printf("Domain undefined successfully")
	return nil
}

// checkVolumesAbsent fails if the VM's boot volume, a data volume or its
// cloud-init volume already exists in its pool: a leftover of an earlier VM
// of the same name, or a volume made by other means, which create must
// neither reuse nor delete. Input volumes are checked by checkInputVolumes.
func checkVolumesAbsent(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager) error {
	type volume struct{ kind, name string }
	volumes := []volume{{"boot", getBootVolumeName(vm)}}
	for _, d := range vm.Spec.DataDisks {
		volumes = append(volumes, volume{"data", getDataVolumeName(vm, d.Device)})
	}
	if vm.Spec.CloudInit != nil {
		volumes = append(volumes, volume{"cloud-init", getCloudInitVolumeName(vm)})
	}
	pool := getStoragePool(vm)
	for _, v := range volumes {
		exists, err := sm.VolumeExists(ctx, pool, v.name)
		if err != nil {
			return fmt.Errorf("failed to check %s volume %s: %w", v.kind, v.name, err)
		}
		if exists {
			return fmt.Errorf("%s volume already exists: %s/%s", v.kind, pool, v.name)
		}
	}
	return nil
}
