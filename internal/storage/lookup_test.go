package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"
)

// failingLookup is the mock with every volume lookup failing for a reason
// other than "no such volume".
type failingLookup struct {
	*mockLibvirtClient
}

func (f failingLookup) StorageVolLookupByName(libvirt.StoragePool, string) (libvirt.StorageVol, error) {
	return libvirt.StorageVol{}, errors.New("connection reset by peer")
}

// TestManager_VolumeExists_LookupFails: only libvirt's "no such volume" means
// absent; any other lookup failure is an error, not "absent", which would
// let create go ahead over a volume that is there.
func TestManager_VolumeExists_LookupFails(t *testing.T) {
	client := newMockLibvirtClient()
	if err := NewManager(client).CreatePool(context.Background(), "p", PoolTypeDir, "/p"); err != nil {
		t.Fatal(err)
	}
	exists, err := NewManager(client).VolumeExists(context.Background(), "p", "missing")
	if err != nil || exists {
		t.Errorf("missing volume: exists %v, err %v; want false, nil", exists, err)
	}
	exists, err = NewManager(failingLookup{client}).VolumeExists(context.Background(), "p", "v")
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("failed lookup: exists %v, err %v; want the error", exists, err)
	}
}

// TestManager_ListVolumes_Format: listed volumes carry their format, as
// image list and image info show it.
func TestManager_ListVolumes_Format(t *testing.T) {
	client := newMockLibvirtClient()
	mgr := NewManager(client)
	ctx := context.Background()
	if err := mgr.CreatePool(ctx, "p", PoolTypeDir, "/p"); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []VolumeSpec{
		{Name: "a.qcow2", Type: VolumeTypeBaseImage, Format: VolumeFormatQCOW2, CapacityGB: 1},
		{Name: "b.raw", Type: VolumeTypeBaseImage, Format: VolumeFormatRaw, CapacityBytes: 512},
	} {
		if err := mgr.CreateVolume(ctx, "p", spec); err != nil {
			t.Fatal(err)
		}
	}
	vols, err := mgr.ListVolumes(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]VolumeFormat{}
	for _, v := range vols {
		got[v.Name] = v.Format
	}
	if got["a.qcow2"] != VolumeFormatQCOW2 || got["b.raw"] != VolumeFormatRaw {
		t.Errorf("formats %v, want a.qcow2 qcow2 and b.raw raw", got)
	}
}

// TestManager_DeleteImage_Refuses: image delete refuses a Secure Boot key
// profile and an image that backs a volume in another pool (a VM's boot
// disk); an unused image is deleted.
func TestManager_DeleteImage_Refuses(t *testing.T) {
	ctx := context.Background()
	client, mgr := newImagesManager(t)
	if err := mgr.CreatePool(ctx, DefaultVMsPool, PoolTypeDir, "/vms"); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []VolumeSpec{
		{Name: "used.qcow2", Type: VolumeTypeBaseImage, Format: VolumeFormatQCOW2, CapacityGB: 1},
		{Name: "unused.qcow2", Type: VolumeTypeBaseImage, Format: VolumeFormatQCOW2, CapacityGB: 1},
		{Name: "varstore_corp_x86_64.fd", Type: VolumeTypeVarstore, Format: VolumeFormatRaw, CapacityBytes: 540672},
	} {
		if err := mgr.CreateVolume(ctx, DefaultImagesPool, spec); err != nil {
			t.Fatal(err)
		}
	}
	usedPath, err := mgr.GetImagePath(ctx, "used.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.CreateVolume(ctx, DefaultVMsPool, VolumeSpec{Name: "web_boot.qcow2", Type: VolumeTypeBoot,
		Format: VolumeFormatQCOW2, CapacityGB: 2, BackingVolume: usedPath}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteImage(ctx, "used.qcow2"); err == nil || !strings.Contains(err.Error(), "foundry-vms/web_boot.qcow2") {
		t.Errorf("image in use: got %v, want an error naming the volume that uses it", err)
	}
	if err := mgr.DeleteImage(ctx, "varstore_corp_x86_64.fd"); err == nil || !strings.Contains(err.Error(), "key profile") {
		t.Errorf("key profile: got %v, want a key profile error", err)
	}
	if err := mgr.DeleteImage(ctx, "unused.qcow2"); err != nil {
		t.Errorf("unused image: %v", err)
	}
	for _, name := range []string{"used.qcow2", "varstore_corp_x86_64.fd"} {
		if _, ok := client.volumes[DefaultImagesPool][name]; !ok {
			t.Errorf("%s was deleted", name)
		}
	}
	if _, ok := client.volumes[DefaultImagesPool]["unused.qcow2"]; ok {
		t.Error("unused.qcow2 was not deleted")
	}
}
