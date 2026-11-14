package vm

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/cloudinit"
	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/storage"
)

// maxInputDiskBytes bounds the files of one input disk: each image is built in
// memory and uploaded in one stream, and input disks are meant for a few small
// files (a config, a document), not for payloads, which belong in an image.
const maxInputDiskBytes = 16 << 20

// inputImage is a built input disk: its volume name and ISO image.
type inputImage struct {
	volume string
	data   []byte
}

// buildInputImages reads the files of the VM's input disks (spec.inputDisks)
// and builds each disk's ISO-9660 image, labelled as the spec says. create
// runs it before any storage is created, so a missing or oversized file fails
// without anything to clean up.
func buildInputImages(vm *v1alpha1.VirtualMachine) ([]inputImage, error) {
	images := make([]inputImage, 0, len(vm.Spec.InputDisks))
	for i, d := range vm.Spec.InputDisks {
		files := make([]cloudinit.ISOFile, 0, len(d.Files))
		total := 0
		for j, f := range d.Files {
			data := []byte(f.String)
			if f.File != "" {
				var err error
				if data, err = readInputFile(f.File, maxInputDiskBytes-total); err != nil {
					return nil, fmt.Errorf("spec.inputDisks[%d].files[%d]: %w", i, j, err)
				}
			}
			total += len(data)
			if total > maxInputDiskBytes {
				return nil, fmt.Errorf("spec.inputDisks[%d] (label %s): files are larger than %d MiB", i, d.Label, maxInputDiskBytes>>20)
			}
			files = append(files, cloudinit.ISOFile{Name: f.Name, Data: data})
		}
		iso, err := cloudinit.BuildISO(d.Label, files)
		if err != nil {
			return nil, fmt.Errorf("failed to build input disk %s: %w", d.Label, err)
		}
		images = append(images, inputImage{volume: foundrylibvirt.GetInputVolumeName(vm, d.Label), data: iso})
	}
	return images, nil
}

// readInputFile reads a regular file, at most limit bytes of it and one more
// (so the caller sees it is too large without reading all of it). Anything
// else is refused: a device such as /dev/zero would never end, and opening a
// FIFO would wait for a writer (O_NONBLOCK makes that open return at once, so
// the check after it can refuse the FIFO).
func readInputFile(path string, limit int) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return io.ReadAll(io.LimitReader(f, int64(limit)+1))
}

// createInputVolumes creates a raw volume of each input image's exact size in
// the VM's pool and uploads the image into it (StorageVolCreateXML, then
// StorageVolUpload). Each volume it makes is added to made, so that on
// failure the caller's cleanup deletes it with the VM's other volumes.
func createInputVolumes(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager, images []inputImage, made *[]string) error {
	pool := getStoragePool(vm)
	for _, img := range images {
		log.Printf("Creating input disk volume %s (%d bytes)...", img.volume, len(img.data))
		spec := storage.VolumeSpec{
			Name:          img.volume,
			Type:          storage.VolumeTypeInput,
			Format:        storage.VolumeFormatRaw,
			CapacityBytes: uint64(len(img.data)),
		}
		if err := sm.CreateVolume(ctx, pool, spec); err != nil {
			return fmt.Errorf("failed to create input volume %s: %w", img.volume, err)
		}
		*made = append(*made, img.volume)
		if err := sm.WriteVolumeData(ctx, pool, img.volume, img.data); err != nil {
			return fmt.Errorf("failed to write input volume %s: %w", img.volume, err)
		}
	}
	return nil
}

// checkInputVolumes fails if a volume an input disk would be created in
// already exists, as the boot volume check does: a leftover of an earlier VM
// of the same name, which create must neither reuse nor delete.
func checkInputVolumes(ctx context.Context, vm *v1alpha1.VirtualMachine, sm storageManager, images []inputImage) error {
	pool := getStoragePool(vm)
	for _, img := range images {
		exists, err := sm.VolumeExists(ctx, pool, img.volume)
		if err != nil {
			return fmt.Errorf("failed to check input volume %s: %w", img.volume, err)
		}
		if exists {
			return fmt.Errorf("input volume already exists: %s/%s", pool, img.volume)
		}
	}
	return nil
}
