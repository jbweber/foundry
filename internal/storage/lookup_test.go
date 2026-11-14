package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
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

// failingDelete is the mock with one volume that cannot be deleted.
type failingDelete struct {
	*mockLibvirtClient
	name string
}

func (f failingDelete) StorageVolDelete(vol libvirt.StorageVol, flags libvirt.StorageVolDeleteFlags) error {
	if vol.Name == f.name {
		return errors.New("Permission denied")
	}
	return f.mockLibvirtClient.StorageVolDelete(vol, flags)
}

// TestManager_DeletePool_ForceReportsFailures: pool delete --force that
// cannot delete a volume says so and keeps the pool, rather than reporting
// success and leaving the file behind with no pool.
func TestManager_DeletePool_ForceReportsFailures(t *testing.T) {
	ctx := context.Background()
	client := newMockLibvirtClient()
	mgr := NewManager(client)
	if err := mgr.CreatePool(ctx, "scratch", PoolTypeDir, "/scratch"); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.qcow2", "stuck.qcow2"} {
		if err := mgr.CreateVolume(ctx, "scratch", VolumeSpec{Name: n, Type: VolumeTypeData, Format: VolumeFormatQCOW2, CapacityGB: 1}); err != nil {
			t.Fatal(err)
		}
	}
	err := NewManager(failingDelete{client, "stuck.qcow2"}).DeletePool(ctx, "scratch", true)
	if err == nil || !strings.Contains(err.Error(), "stuck.qcow2") || !strings.Contains(err.Error(), "kept") {
		t.Fatalf("got %v, want an error naming stuck.qcow2 and keeping the pool", err)
	}
	if _, ok := client.pools["scratch"]; !ok {
		t.Error("pool undefined although a volume could not be deleted")
	}
}

// failingUpload is the mock with the next upload failing.
type failingUpload struct {
	*mockLibvirtClient
	fail *int // uploads still to fail
}

func (f failingUpload) StorageVolUpload(vol libvirt.StorageVol, r io.Reader, offset, length uint64, flags libvirt.StorageVolUploadFlags) error {
	if *f.fail > 0 {
		*f.fail--
		return errors.New("stream aborted")
	}
	return f.mockLibvirtClient.StorageVolUpload(vol, r, offset, length, flags)
}

// TestManager_ImportVarstore_ReplaceKeepsOldOnFailure: a replace whose
// upload fails puts the old profile back, rather than leaving none.
func TestManager_ImportVarstore_ReplaceKeepsOldOnFailure(t *testing.T) {
	ctx := context.Background()
	mgr, client := newVarstoreTestManager(t)
	old := testVarstore(8192)
	if err := mgr.ImportVarstore(ctx, "my-keys", "x86_64", old, false); err != nil {
		t.Fatal(err)
	}
	fail := 1
	err := NewManager(failingUpload{client, &fail}).ImportVarstore(ctx, "my-keys", "x86_64", testVarstore(4096), true)
	if err == nil || !strings.Contains(err.Error(), "old varstore was put back") {
		t.Fatalf("got %v, want an upload error with the old varstore put back", err)
	}
	vol := client.volumes[DefaultImagesPool]["varstore_my-keys_x86_64.fd"]
	if vol == nil || !bytes.Equal(vol.data, old) {
		t.Error("the old varstore is not in place after the failed replace")
	}
}
