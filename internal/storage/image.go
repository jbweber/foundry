package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/jbweber/foundry/internal/naming"
)

// Image name extensions ImportImage accepts, each with the content it
// requires.
const (
	ImageExtQCOW2 = ".qcow2" // QCOW2 magic
	ImageExtRaw   = ".raw"   // a boot sector signature (a bootable disk image)
)

// CheckImageName refuses an image name without one of the accepted
// extensions, before anything is read.
func CheckImageName(imageName string) error {
	switch filepath.Ext(imageName) {
	case ImageExtQCOW2, ImageExtRaw:
		return nil
	}
	return fmt.Errorf("image name must have .qcow2 or .raw extension (got: %q)", imageName)
}

// ImportImage imports a base image from a local file into the foundry-images
// pool.
//
// The name's extension says what the file must be: .qcow2 a QCOW2 image,
// .raw a bootable raw disk image. A qcow2 image that would make qemu read
// another file (a backing file, an external data file) is refused
// (checkQCOW2Header). The file must be a regular file (a symlink to one is
// followed) or a pipe, which is read as ImportImageFromReader reads. A
// directory or a device is refused before it is opened: its size is not the
// size of what it would stream, and opening some devices does something. The
// file is streamed to libvirt, not read
// into memory, and if it ends before the size it had when it was opened, the
// import fails. On any failure after the volume is created, the volume is
// deleted.
func (m *Manager) ImportImage(ctx context.Context, filePath, imageName string) error {
	if err := CheckImageName(imageName); err != nil {
		return err
	}
	if err := checkImageSource(filePath); err != nil {
		return err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open image file: %w", err)
	}
	defer func() { _ = f.Close() }()
	// Stat the open file as well, so the checks and the upload see the file
	// that was opened even if the path was replaced meanwhile
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat image file: %w", err)
	}
	switch {
	case info.Mode().IsRegular():
		return m.importImageFile(ctx, f, info.Size(), imageName)
	case info.Mode().Type() == os.ModeNamedPipe:
		// A pipe (such as a shell's <(...)) has no size and cannot seek:
		// read it as standard input is read
		return m.ImportImageFromReader(ctx, f, imageName)
	}
	return notImageSource(filePath, info)
}

// checkImageSource refuses a path that is neither a regular file nor a pipe
// (after symlinks).
func checkImageSource(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat image file: %w", err)
	}
	if info.Mode().IsRegular() || info.Mode().Type() == os.ModeNamedPipe {
		return nil
	}
	return notImageSource(filePath, info)
}

func notImageSource(filePath string, info os.FileInfo) error {
	return fmt.Errorf("image file %s is not a regular file or a pipe (%s)", filePath, info.Mode().Type())
}

// ImportImageFromReader imports a base image read from r (e.g. standard
// input) into the foundry-images pool, with the same checks as ImportImage.
//
// The image is spooled to a temporary file (in os.TempDir, i.e. $TMPDIR or
// /tmp) and imported from it, rather than streamed straight into the
// volume: libvirt needs the volume, with its capacity, before the upload
// (StorageVolCreateXML, then StorageVolUpload), and a stream's length is
// only known at its end; the format checks also read at offsets of a
// seekable file. Spooling needs no size from the caller. The cost is a
// temporary copy of the image on the machine foundry runs on, as large as
// the image: a $TMPDIR without the room fails the import before any volume
// is made.
//
// The spool file is removed from its directory as soon as it is created and
// used through its open descriptor, so it is gone however foundry ends,
// killed included; its space is freed when foundry exits.
func (m *Manager) ImportImageFromReader(ctx context.Context, r io.Reader, imageName string) error {
	// Check the name before reading anything: a bad name should not cost a
	// copy of the image
	if err := CheckImageName(imageName); err != nil {
		return err
	}

	f, err := os.CreateTemp("", "foundry-image-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for the image: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := os.Remove(f.Name()); err != nil {
		// Not unlinked now: remove it when the import ends instead
		defer func() { _ = os.Remove(f.Name()) }()
	}

	src := &sizedReader{ctx: ctx, r: r, left: -1}
	size, err := io.Copy(f, src)
	if readErr := src.failure(); readErr != nil {
		return fmt.Errorf("failed to read image: %w", readErr)
	}
	if err != nil {
		return fmt.Errorf("failed to write the image to a temporary file in %s: %w", os.TempDir(), err)
	}
	return m.importImageFile(ctx, f, size, imageName)
}

// importImageFile checks the first size bytes of f against imageName,
// creates the image's volume and streams them into it.
func (m *Manager) importImageFile(ctx context.Context, f io.ReaderAt, size int64, imageName string) error {
	spec, err := imageVolumeSpec(imageName, io.NewSectionReader(f, 0, size), size)
	if err != nil {
		return err
	}

	if err := m.CreateVolume(ctx, DefaultImagesPool, spec); err != nil {
		return fmt.Errorf("failed to create image volume: %w", err)
	}

	// The volume is this call's from here on: delete it on any failure, so
	// no partial image is left under the name a VM would use
	src := &sizedReader{ctx: ctx, r: io.NewSectionReader(f, 0, size), left: size}
	err = m.WriteVolumeFrom(ctx, DefaultImagesPool, imageName, src, uint64(size))
	if readErr := src.failure(); readErr != nil {
		err = fmt.Errorf("failed to read image: %w", readErr)
	} else if err != nil {
		err = fmt.Errorf("failed to upload image data: %w", err)
	}
	if err != nil {
		if delErr := m.DeleteVolume(context.WithoutCancel(ctx), DefaultImagesPool, imageName); delErr != nil {
			return fmt.Errorf("%w; the partial volume %s/%s is left behind: %v", err, DefaultImagesPool, imageName, delErr)
		}
		return err
	}
	return nil
}

// imageVolumeSpec checks that an image's content is what its name's
// extension says, and returns the spec of the volume for it. content is the
// image; size is its length.
func imageVolumeSpec(imageName string, content *io.SectionReader, size int64) (VolumeSpec, error) {
	spec := VolumeSpec{
		Name: imageName,
		Type: VolumeTypeBaseImage,
	}

	// Detect actual format from file content
	detectedFormat, err := detectImageFormat(content)
	if err != nil {
		return spec, fmt.Errorf("failed to detect image format: %w", err)
	}

	// Determine expected format from image name extension
	expectedFormat := VolumeFormatRaw
	if filepath.Ext(imageName) == ImageExtQCOW2 {
		expectedFormat = VolumeFormatQCOW2
	}

	// Validate format matches extension
	if detectedFormat != expectedFormat {
		return spec, fmt.Errorf("format mismatch: file is %q but image name %q expects %q",
			detectedFormat, imageName, expectedFormat)
	}
	spec.Format = detectedFormat
	if detectedFormat == VolumeFormatQCOW2 {
		head := make([]byte, min(size, qcow2HeadBytes))
		if _, err := content.ReadAt(head, 0); err != nil {
			return spec, fmt.Errorf("failed to read the qcow2 header: %w", err)
		}
		if err := checkQCOW2Header(head); err != nil {
			return spec, err
		}
		// Size in GiB, rounded up: the upload replaces the header qemu-img
		// wrote, and the guest-visible size with it
		spec.CapacityGB = uint64(size/(1024*1024*1024)) + 1
		return spec, nil
	}
	// A raw volume's file is its capacity long, so exactly the image's size:
	// no padding the guest would see as disk, and read back to its end the
	// volume hashes as the image does
	spec.CapacityBytes = uint64(size)
	return spec, nil
}

// sizedReader reads exactly left bytes from r (all of r when left is
// negative), and remembers why reading failed: go-libvirt reports a failed
// upload with libvirt's answer to the aborted stream, not the reader's
// error. Content that ends early is io.ErrUnexpectedEOF; a ctx that ends
// stops the read with ctx's error.
type sizedReader struct {
	ctx  context.Context
	r    io.Reader
	left int64

	mu  sync.Mutex
	err error
}

func (s *sizedReader) Read(p []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, s.fail(err)
	}
	if s.left == 0 {
		return 0, io.EOF
	}
	if s.left > 0 && int64(len(p)) > s.left {
		p = p[:s.left]
	}
	n, err := s.r.Read(p)
	if s.left > 0 {
		s.left -= int64(n)
	}
	switch {
	case errors.Is(err, io.EOF) && s.left > 0:
		return n, s.fail(io.ErrUnexpectedEOF)
	case err != nil && !errors.Is(err, io.EOF):
		return n, s.fail(err)
	}
	return n, err
}

func (s *sizedReader) fail(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
	return err
}

func (s *sizedReader) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// PullImage downloads and imports a base image from a URL.
// This is a placeholder for future implementation.
func (m *Manager) PullImage(ctx context.Context, url, imageName, checksum string) error {
	return fmt.Errorf("pull image is not yet implemented")
}

// ListImages lists all base images in the foundry-images pool. Secure Boot
// key profile varstores share the pool but are not images; ListVarstores
// lists them.
func (m *Manager) ListImages(ctx context.Context) ([]VolumeInfo, error) {
	vols, err := m.ListVolumes(ctx, DefaultImagesPool)
	if err != nil {
		return nil, err
	}
	images := vols[:0]
	for _, v := range vols {
		if _, _, ok := naming.ParseVolumeNameVarstore(v.Name); !ok {
			images = append(images, v)
		}
	}
	return images, nil
}

// DeleteImage deletes a base image from the foundry-images pool.
// If force is true, the image is deleted even if it's being used as a backing file.
func (m *Manager) DeleteImage(ctx context.Context, imageName string, force bool) error {
	// TODO: If force is false, check if the image is being used as a backing file
	// This would require iterating through all VMs and checking their backing stores
	// For now, we just delete the image regardless of force flag

	// Note: force parameter is reserved for future use when we implement backing file checks
	_ = force

	return m.DeleteVolume(ctx, DefaultImagesPool, imageName)
}

// GetImagePath gets the full filesystem path for a base image.
func (m *Manager) GetImagePath(ctx context.Context, imageName string) (string, error) {
	return m.GetVolumePath(ctx, DefaultImagesPool, imageName)
}

// ImageExists checks if a base image exists in the foundry-images pool.
func (m *Manager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	return m.VolumeExists(ctx, DefaultImagesPool, imageName)
}
