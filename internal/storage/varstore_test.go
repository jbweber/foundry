package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

// testVarstore builds a minimal raw edk2 varstore of size bytes: a firmware
// volume header with the variable store GUID, the "_FVH" signature and an
// FvLength of the whole file.
func testVarstore(size int) []byte {
	data := make([]byte, size)
	copy(data[16:32], systemNvDataFvGUID)
	binary.LittleEndian.PutUint64(data[32:40], uint64(size))
	copy(data[40:44], "_FVH")
	return data
}

func TestValidateVarstore(t *testing.T) {
	wrongGUID := testVarstore(4096)
	wrongGUID[16] ^= 0xff

	longFV := testVarstore(4096)
	binary.LittleEndian.PutUint64(longFV[32:40], 8192)

	zeroFV := testVarstore(4096)
	binary.LittleEndian.PutUint64(zeroFV[32:40], 0)

	qcow2 := make([]byte, 4096)
	copy(qcow2, "QFI\xfb")

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "x86_64 size", data: testVarstore(540672)},
		{name: "one block", data: testVarstore(4096)},
		{name: "empty", data: nil, wantErr: "empty"},
		{name: "partial block", data: testVarstore(4096)[:4000], wantErr: "4 KiB blocks"},
		{name: "too large", data: make([]byte, MaxVarstoreSize+4096), wantErr: "more than"},
		{name: "qcow2", data: qcow2, wantErr: "_FVH"},
		{name: "not a variable store", data: wrongGUID, wantErr: "gEfiSystemNvDataFvGuid"},
		{name: "firmware volume longer than the file", data: longFV, wantErr: "does not fit"},
		{name: "zero firmware volume length", data: zeroFV, wantErr: "does not fit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVarstore(tt.data)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadVarstore(t *testing.T) {
	data, err := ReadVarstore(bytes.NewReader(testVarstore(8192)))
	if err != nil || len(data) != 8192 {
		t.Fatalf("ReadVarstore() = %d bytes, %v; want 8192 bytes", len(data), err)
	}
	// A stream longer than the limit is refused without reading it all.
	if _, err := ReadVarstore(bytes.NewReader(make([]byte, MaxVarstoreSize+8192))); err == nil {
		t.Fatal("expected an error for an oversized varstore")
	}
}

func newVarstoreTestManager(t *testing.T) (*Manager, *mockLibvirtClient) {
	t.Helper()
	client := newMockLibvirtClient()
	mgr := NewManager(client)
	if err := mgr.CreatePool(context.Background(), DefaultImagesPool, PoolTypeDir, DefaultImagesPath); err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	return mgr, client
}

func TestManager_ImportVarstore(t *testing.T) {
	ctx := context.Background()
	mgr, client := newVarstoreTestManager(t)
	data := testVarstore(8192)

	if err := mgr.ImportVarstore(ctx, "my-keys", "x86_64", data, false); err != nil {
		t.Fatalf("ImportVarstore: %v", err)
	}
	vol := client.volumes[DefaultImagesPool]["varstore_my-keys_x86_64.fd"]
	if vol == nil {
		t.Fatal("varstore volume not created")
	}
	if !bytes.Equal(vol.data, data) {
		t.Error("varstore volume data differs from the imported file")
	}

	// A second import of the same profile and architecture needs replace.
	if err := mgr.ImportVarstore(ctx, "my-keys", "x86_64", data, false); err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("second import error = %v, want an already exists error", err)
	}
	other := testVarstore(4096)
	if err := mgr.ImportVarstore(ctx, "my-keys", "x86_64", other, true); err != nil {
		t.Fatalf("ImportVarstore with replace: %v", err)
	}
	if got := client.volumes[DefaultImagesPool]["varstore_my-keys_x86_64.fd"].data; !bytes.Equal(got, other) {
		t.Error("replace did not upload the new varstore")
	}

	// An invalid file is refused before any volume is made.
	if err := mgr.ImportVarstore(ctx, "bad", "x86_64", make([]byte, 4096), false); err == nil {
		t.Fatal("expected an error for an invalid varstore")
	}
	if _, ok := client.volumes[DefaultImagesPool]["varstore_bad_x86_64.fd"]; ok {
		t.Error("volume created for an invalid varstore")
	}
}

func TestManager_ListAndDeleteVarstores(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newVarstoreTestManager(t)
	for _, p := range []struct{ profile, arch string }{{"zeta", "x86_64"}, {"alpha", "x86_64"}, {"alpha", "aarch64"}} {
		if err := mgr.ImportVarstore(ctx, p.profile, p.arch, testVarstore(4096), false); err != nil {
			t.Fatalf("ImportVarstore(%s, %s): %v", p.profile, p.arch, err)
		}
	}
	if err := mgr.CreateVolume(ctx, DefaultImagesPool, VolumeSpec{Name: "fedora-43.qcow2", Type: VolumeTypeBaseImage, Format: VolumeFormatQCOW2, CapacityGB: 5}); err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}

	stores, err := mgr.ListVarstores(ctx)
	if err != nil {
		t.Fatalf("ListVarstores: %v", err)
	}
	var got []string
	for _, s := range stores {
		got = append(got, s.Profile+"/"+s.Arch)
	}
	if want := "alpha/aarch64 alpha/x86_64 zeta/x86_64"; strings.Join(got, " ") != want {
		t.Errorf("ListVarstores() = %v, want %s", got, want)
	}

	images, err := mgr.ListImages(ctx)
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 1 || images[0].Name != "fedora-43.qcow2" {
		t.Errorf("ListImages() = %v, want only fedora-43.qcow2", images)
	}

	if err := mgr.DeleteVarstore(ctx, "alpha", "x86_64"); err != nil {
		t.Fatalf("DeleteVarstore: %v", err)
	}
	if err := mgr.DeleteVarstore(ctx, "alpha", "x86_64"); err == nil {
		t.Error("expected an error deleting a missing varstore")
	}
	stores, _ = mgr.ListVarstores(ctx)
	if len(stores) != 2 {
		t.Errorf("ListVarstores() after delete = %d entries, want 2", len(stores))
	}
}

func TestGenerateVolumeXML_CapacityBytes(t *testing.T) {
	xml, err := generateVolumeXML("", VolumeSpec{Name: "v.fd", Type: VolumeTypeVarstore, Format: VolumeFormatRaw, CapacityBytes: 540672}, nil)
	if err != nil {
		t.Fatalf("generateVolumeXML: %v", err)
	}
	if !strings.Contains(xml, `<capacity unit="B">540672</capacity>`) {
		t.Errorf("capacity not exact in:\n%s", xml)
	}
}
