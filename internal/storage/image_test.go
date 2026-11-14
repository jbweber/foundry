package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestManager_ImportImage(t *testing.T) {
	tmpDir := t.TempDir()

	// Helper to create QCOW2 test file
	createQCOW2 := func(path string) error {
		return os.WriteFile(path, qcow2Fixture(512), 0644)
	}

	// Helper to create bootable RAW test file
	createBootableRAW := func(path string) error {
		data := make([]byte, 512)
		data[510] = 0x55
		data[511] = 0xaa
		return os.WriteFile(path, data, 0644)
	}

	// Create test files
	qcow2Path := filepath.Join(tmpDir, "test-image.qcow2")
	if err := createQCOW2(qcow2Path); err != nil {
		t.Fatalf("Failed to create QCOW2 test file: %v", err)
	}

	rawPath := filepath.Join(tmpDir, "test-image.raw")
	if err := createBootableRAW(rawPath); err != nil {
		t.Fatalf("Failed to create RAW test file: %v", err)
	}

	// Create misnamed file (QCOW2 with .raw extension)
	misnamedPath := filepath.Join(tmpDir, "misnamed.raw")
	if err := createQCOW2(misnamedPath); err != nil {
		t.Fatalf("Failed to create misnamed test file: %v", err)
	}

	// Create non-bootable file
	nonBootablePath := filepath.Join(tmpDir, "non-bootable.raw")
	if err := os.WriteFile(nonBootablePath, make([]byte, 512), 0644); err != nil {
		t.Fatalf("Failed to create non-bootable test file: %v", err)
	}

	tests := []struct {
		name      string
		filePath  string
		imageName string
		setup     func(*mockLibvirtClient, *Manager)
		wantErr   bool
		errMsg    string // Expected error substring
	}{
		{
			name:      "import qcow2 image",
			filePath:  qcow2Path,
			imageName: "fedora-43.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: false,
		},
		{
			name:      "import bootable raw image",
			filePath:  rawPath,
			imageName: "ubuntu-24.04.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: false,
		},
		{
			name:      "reject image name without extension",
			filePath:  qcow2Path,
			imageName: "fedora-43",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "must have .qcow2 or .raw extension",
		},
		{
			name:      "reject image name with wrong extension",
			filePath:  qcow2Path,
			imageName: "fedora-43.img",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "must have .qcow2 or .raw extension",
		},
		{
			name:      "reject format mismatch (qcow2 file, raw name)",
			filePath:  qcow2Path,
			imageName: "fedora-43.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject format mismatch (raw file, qcow2 name)",
			filePath:  rawPath,
			imageName: "ubuntu-24.04.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject misnamed file",
			filePath:  misnamedPath,
			imageName: "misnamed.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "format mismatch",
		},
		{
			name:      "reject non-bootable raw file",
			filePath:  nonBootablePath,
			imageName: "non-bootable.raw",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
			errMsg:  "unsupported or invalid image",
		},
		{
			name:      "import non-existent file",
			filePath:  "/nonexistent/image.qcow2",
			imageName: "fedora-43.qcow2",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
		{
			name:      "pool not found",
			filePath:  qcow2Path,
			imageName: "fedora-43.qcow2",
			setup:     func(m *mockLibvirtClient, mgr *Manager) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.ImportImage(context.Background(), tt.filePath, tt.imageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ImportImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If error expected, verify error message contains expected substring
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !contains(err.Error(), tt.errMsg) {
					t.Errorf("ImportImage() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			// If successful, verify the image was created
			if err == nil {
				exists, _ := mgr.ImageExists(context.Background(), tt.imageName)
				if !exists {
					t.Errorf("Image %s not found after ImportImage()", tt.imageName)
				}

				// Verify data was uploaded
				vol := mockClient.volumes[DefaultImagesPool][tt.imageName]
				if len(vol.data) == 0 {
					t.Errorf("Image data not uploaded")
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestManager_ListImages(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)

	// Create some images
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       "fedora-43",
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       "ubuntu-24.04",
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 8,
	})

	images, err := mgr.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}

	if len(images) != 2 {
		t.Errorf("ListImages() returned %d images, want 2", len(images))
	}
}

func TestManager_DeleteImage(t *testing.T) {
	tests := []struct {
		name      string
		imageName string
		setup     func(*mockLibvirtClient, *Manager)
		wantErr   bool
	}{
		{
			name:      "delete existing image",
			imageName: "test-image",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
				_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
					Name:       "test-image",
					Type:       VolumeTypeBaseImage,
					Format:     VolumeFormatQCOW2,
					CapacityGB: 10,
				})
			},
			wantErr: false,
		},
		{
			name:      "delete non-existent image",
			imageName: "nonexistent",
			setup: func(m *mockLibvirtClient, mgr *Manager) {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockLibvirtClient()
			mgr := NewManager(mockClient)
			tt.setup(mockClient, mgr)

			err := mgr.DeleteImage(context.Background(), tt.imageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_GetImagePath(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool and image
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
	imageName := "fedora-43"
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       imageName,
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})

	path, err := mgr.GetImagePath(context.Background(), imageName)
	if err != nil {
		t.Fatalf("GetImagePath() error = %v", err)
	}

	if path == "" {
		t.Errorf("GetImagePath() returned empty path")
	}
}

func TestManager_ImageExists(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// Create images pool and image
	_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
	imageName := "fedora-43"
	_ = mgr.CreateVolume(context.Background(), DefaultImagesPool, VolumeSpec{
		Name:       imageName,
		Type:       VolumeTypeBaseImage,
		Format:     VolumeFormatQCOW2,
		CapacityGB: 10,
	})

	// Check existing image
	exists, err := mgr.ImageExists(context.Background(), imageName)
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Errorf("ImageExists() = false, want true")
	}

	// Check non-existent image
	exists, err = mgr.ImageExists(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if exists {
		t.Errorf("ImageExists() = true, want false")
	}
}

func TestManager_PullImage(t *testing.T) {
	mockClient := newMockLibvirtClient()
	mgr := NewManager(mockClient)

	// PullImage is not yet implemented
	err := mgr.PullImage(context.Background(), "http://example.com/image.qcow2", "test-image", "")
	if err == nil {
		t.Errorf("PullImage() should return error for unimplemented feature")
	}
}

// bootableRaw is a raw disk image of size bytes (at least 512) with a boot
// sector signature.
func bootableRaw(size int) []byte {
	data := make([]byte, size)
	data[510], data[511] = 0x55, 0xaa
	return data
}

// newImagesManager returns a manager over a mock with the images pool.
func newImagesManager(t *testing.T) (*mockLibvirtClient, *Manager) {
	t.Helper()
	client := newMockLibvirtClient()
	mgr := NewManager(client)
	if err := mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath); err != nil {
		t.Fatal(err)
	}
	return client, mgr
}

// A raw image's volume is exactly the file's size, not rounded up to whole
// GiB: the guest sees no padding, and the volume hashes as the file does.
func TestManager_ImportImage_RawExactSize(t *testing.T) {
	data := bootableRaw(3*512 + 7)
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	client, mgr := newImagesManager(t)
	if err := mgr.ImportImage(context.Background(), path, "disk.raw"); err != nil {
		t.Fatalf("ImportImage() error = %v", err)
	}
	vol := client.volumes[DefaultImagesPool]["disk.raw"]
	if !bytes.Equal(vol.data, data) {
		t.Errorf("uploaded %d bytes, want the file's %d", len(vol.data), len(data))
	}
	if want := fmt.Sprintf(`<capacity unit="B">%d</capacity>`, len(data)); !strings.Contains(vol.xml, want) {
		t.Errorf("volume XML lacks %s: %s", want, vol.xml)
	}
}

// Only a regular file (or a symlink to one) is imported from a path: a
// directory's or device's size is not the size of what it would stream.
func TestManager_ImportImage_FileTypes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "disk.raw")
	if err := os.WriteFile(file, bootableRaw(512), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.raw")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, path, wantErr string
	}{
		{"symlink to a regular file", link, ""},
		{"directory", dir, "not a regular file or a pipe"},
		{"character device", "/dev/null", "not a regular file or a pipe"},
		{"missing", filepath.Join(dir, "none.raw"), "no such file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, mgr := newImagesManager(t)
			err := mgr.ImportImage(context.Background(), tt.path, "disk.raw")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ImportImage() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ImportImage() error = %v, want %q", err, tt.wantErr)
			}
			if len(client.volumes[DefaultImagesPool]) != 0 {
				t.Error("a volume was created for a refused source")
			}
		})
	}
}

// shortReaderAt holds fewer bytes than the size the import is told: a file
// that shrank after it was opened.
type shortReaderAt struct{ data []byte }

func (s shortReaderAt) ReadAt(p []byte, off int64) (int, error) {
	return bytes.NewReader(s.data).ReadAt(p, off)
}

// An image that ends early, or an import that is cancelled, leaves no
// volume: a partial image under the name would boot as if it were whole.
func TestManager_ImportImage_PartialUploadDeleted(t *testing.T) {
	data := bootableRaw(4096)
	t.Run("content shorter than its size", func(t *testing.T) {
		client, mgr := newImagesManager(t)
		err := mgr.importImageFile(context.Background(), shortReaderAt{data[:1024]}, int64(len(data)), "disk.raw")
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("importImageFile() error = %v, want io.ErrUnexpectedEOF", err)
		}
		if _, ok := client.volumes[DefaultImagesPool]["disk.raw"]; ok {
			t.Error("the partial volume is left behind")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		client, mgr := newImagesManager(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := mgr.importImageFile(ctx, bytes.NewReader(data), int64(len(data)), "disk.raw")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("importImageFile() error = %v, want context.Canceled", err)
		}
		if _, ok := client.volumes[DefaultImagesPool]["disk.raw"]; ok {
			t.Error("the partial volume is left behind")
		}
	})
	t.Run("content longer than its size", func(t *testing.T) {
		// Only the size given is sent: a file that grew after it was opened
		// is imported as it was
		client, mgr := newImagesManager(t)
		if err := mgr.importImageFile(context.Background(), bytes.NewReader(data), 1024, "disk.raw"); err != nil {
			t.Fatalf("importImageFile() error = %v", err)
		}
		if got := len(client.volumes[DefaultImagesPool]["disk.raw"].data); got != 1024 {
			t.Errorf("uploaded %d bytes, want 1024", got)
		}
	})
}

func TestManager_ImportImageFromReader(t *testing.T) {
	qcow2 := qcow2Fixture(512)
	raw := bootableRaw(1536)
	backed := qcow2Fixture(512)
	backed[15] = 1 // backing_file_offset

	tests := []struct {
		name      string
		input     []byte
		imageName string
		noPool    bool
		wantErr   string // substring; empty means success
		wantRead  bool   // whether the input should have been consumed
	}{
		{name: "qcow2 from reader", input: qcow2, imageName: "fedora-43.qcow2", wantRead: true},
		{name: "raw from reader", input: raw, imageName: "ubuntu-24.04.raw", wantRead: true},
		{name: "format mismatch", input: qcow2, imageName: "fedora-43.raw", wantErr: "format mismatch", wantRead: true},
		{name: "non-bootable raw", input: make([]byte, 512), imageName: "blank.raw", wantErr: "unsupported or invalid image", wantRead: true},
		{name: "qcow2 with a backing file", input: backed, imageName: "x.qcow2", wantErr: "backing file", wantRead: true},
		{name: "empty input", input: nil, imageName: "empty.qcow2", wantErr: "too small", wantRead: true},
		{name: "bad name fails before reading", input: qcow2, imageName: "fedora-43", wantErr: "must have .qcow2 or .raw extension", wantRead: false},
		{name: "pool not found", input: qcow2, imageName: "fedora-43.qcow2", noPool: true, wantErr: "pool not found", wantRead: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The spool file goes to os.TempDir; point it at a directory the
			// test can check is left empty
			spool := t.TempDir()
			t.Setenv("TMPDIR", spool)

			client := newMockLibvirtClient()
			mgr := NewManager(client)
			if !tt.noPool {
				_ = mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath)
			}

			r := bytes.NewReader(tt.input)
			err := mgr.ImportImageFromReader(context.Background(), r, tt.imageName)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ImportImageFromReader() error = %v", err)
				}
				vol := client.volumes[DefaultImagesPool][tt.imageName]
				if !bytes.Equal(vol.data, tt.input) {
					t.Errorf("uploaded %d bytes, want the %d input bytes", len(vol.data), len(tt.input))
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ImportImageFromReader() error = %v, want error containing %q", err, tt.wantErr)
				}
				if _, ok := client.volumes[DefaultImagesPool][tt.imageName]; ok {
					t.Error("a volume was left for a refused image")
				}
			}

			if read := r.Len() < len(tt.input); len(tt.input) > 0 && read != tt.wantRead {
				t.Errorf("input consumed = %v, want %v", read, tt.wantRead)
			}
			entries, err := os.ReadDir(spool)
			if err != nil {
				t.Fatalf("ReadDir(%s): %v", spool, err)
			}
			if len(entries) != 0 {
				t.Errorf("temporary files left behind: %v", entries)
			}
		})
	}
}

// failingReader returns some bytes, then an error: a stream that breaks.
type failingReader struct{ sent bool }

func (f *failingReader) Read(p []byte) (int, error) {
	if !f.sent {
		f.sent = true
		return copy(p, bootableRaw(512)), nil
	}
	return 0, errors.New("connection reset")
}

// A stream that breaks fails the import before any volume is made, and the
// spool file is gone.
func TestManager_ImportImageFromReader_BrokenStream(t *testing.T) {
	spool := t.TempDir()
	t.Setenv("TMPDIR", spool)
	client, mgr := newImagesManager(t)
	err := mgr.ImportImageFromReader(context.Background(), &failingReader{}, "disk.raw")
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v, want the read's error", err)
	}
	if len(client.volumes[DefaultImagesPool]) != 0 {
		t.Error("a volume was made from a broken stream")
	}
	if entries, _ := os.ReadDir(spool); len(entries) != 0 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

// A named pipe given as the path (a shell's <(...)) is read as standard
// input is.
func TestManager_ImportImage_NamedPipe(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no named pipes here: %v", err)
	}
	data := bootableRaw(2048)
	go func() {
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	client, mgr := newImagesManager(t)
	if err := mgr.ImportImage(context.Background(), fifo, "disk.raw"); err != nil {
		t.Fatalf("ImportImage() error = %v", err)
	}
	if got := client.volumes[DefaultImagesPool]["disk.raw"].data; !bytes.Equal(got, data) {
		t.Errorf("uploaded %d bytes, want the %d written", len(got), len(data))
	}
}

// VolumeSHA256 hashes the volume as stored, to its end.
func TestManager_VolumeSHA256(t *testing.T) {
	client, m := newImagesManager(t)
	ctx := context.Background()
	client.volumes[DefaultImagesPool]["a.raw"] = &mockVolume{name: "a.raw", data: []byte("hello")}
	got, err := m.VolumeSHA256(ctx, DefaultImagesPool, "a.raw")
	// sha256 of "hello"
	if want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"; err != nil || got != want {
		t.Errorf("%s, %v; want %s", got, err, want)
	}
	if _, err := m.VolumeSHA256(ctx, DefaultImagesPool, "none.raw"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("no volume: %v", err)
	}
	if _, err := m.VolumeSHA256(ctx, "no-pool", "a.raw"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("no pool: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.VolumeSHA256(cancelled, DefaultImagesPool, "a.raw"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

// An imported image hashes, read back, as its file does: what a spec's
// imageSHA256, taken with sha256sum from the file, is checked against.
func TestManager_ImportImage_ReadBackHashesAsFile(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"raw", bootableRaw(4096 + 3)},
		{"qcow2", qcow2Fixture(8192)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, mgr := newImagesManager(t)
			name := "disk." + tt.name
			if err := mgr.ImportImageFromReader(context.Background(), bytes.NewReader(tt.data), name); err != nil {
				t.Fatal(err)
			}
			got, err := mgr.VolumeSHA256(context.Background(), DefaultImagesPool, name)
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("%x", sha256.Sum256(tt.data)); got != want {
				t.Errorf("volume sha256 %s, file sha256 %s", got, want)
			}
		})
	}
}
