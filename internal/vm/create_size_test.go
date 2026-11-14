package vm

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateFromConfigWithDeps_BootDiskSmallerThanImage: a boot disk smaller
// than its image's virtual size is refused before anything is made; the
// overlay would cut the guest's disk off in the middle of its partitions.
func TestCreateFromConfigWithDeps_BootDiskSmallerThanImage(t *testing.T) {
	t.Run("image volume", func(t *testing.T) {
		vm := testVMConfig() // sizeGB 20
		vm.Spec.BootDisk.Empty = false
		vm.Spec.BootDisk.Image = "fedora-43.qcow2"
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()
		sm.imageCapacityFunc = func(context.Context, string) (uint64, error) { return 30 << 30, nil }

		err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
		if err == nil || !strings.Contains(err.Error(), "sizeGB is 20, smaller than its image") || !strings.Contains(err.Error(), "at least 30 GiB") {
			t.Fatalf("got %v, want a boot disk size error", err)
		}
		if len(sm.createVolumeCalls) != 0 {
			t.Errorf("made %v before refusing", sm.createVolumeCalls)
		}
	})
	t.Run("image volume of the same size", func(t *testing.T) {
		vm := testVMConfig()
		vm.Spec.BootDisk.Empty = false
		vm.Spec.BootDisk.Image = "fedora-43.qcow2"
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()
		sm.imageCapacityFunc = func(context.Context, string) (uint64, error) { return 20 << 30, nil }
		if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err != nil {
			t.Fatalf("create: %v", err)
		}
	})
	t.Run("host path qcow2", func(t *testing.T) {
		q := make([]byte, 4096)
		copy(q, []byte{0x51, 0x46, 0x49, 0xfb})
		binary.BigEndian.PutUint32(q[4:], 3)       // version
		binary.BigEndian.PutUint64(q[24:], 21<<30) // size: 21 GiB virtual
		binary.BigEndian.PutUint32(q[20:], 16)     // cluster_bits
		binary.BigEndian.PutUint32(q[100:], 104)   // header_length
		path := filepath.Join(t.TempDir(), "big.qcow2")
		if err := os.WriteFile(path, q, 0o600); err != nil {
			t.Fatal(err)
		}
		vm := testVMConfig()
		vm.Spec.BootDisk.Empty = false
		vm.Spec.BootDisk.Image = path
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()

		err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
		if err == nil || !strings.Contains(err.Error(), "smaller than its image") {
			t.Fatalf("got %v, want a boot disk size error", err)
		}
		if len(sm.createVolumeCalls) != 0 {
			t.Errorf("made %v before refusing", sm.createVolumeCalls)
		}
	})
}
