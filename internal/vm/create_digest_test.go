package vm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jbweber/foundry/internal/storage"
)

// spec.bootDisk.imageSHA256 is checked against the image volume create backs
// the boot disk with, before anything is made.
func TestCreateFromConfigWithDeps_ImageSHA256(t *testing.T) {
	const stored = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // the mock's
	other := strings.Repeat("0", 64)
	for _, tt := range []struct {
		name       string
		image      string
		digest     string
		hashErr    error
		wantErr    string // empty: the create succeeds
		wantHashed bool
	}{
		{name: "no digest: not read", image: "fedora-43.qcow2"},
		{name: "matching digest", image: "fedora-43.qcow2", digest: stored, wantHashed: true},
		{name: "pool and volume reference", image: "foundry-images:fedora-43.qcow2", digest: stored, wantHashed: true},
		{name: "different digest", image: "fedora-43.qcow2", digest: other, wantErr: "has sha256 " + stored, wantHashed: true},
		{name: "read fails", image: "fedora-43.qcow2", digest: stored, hashErr: errors.New("stream aborted"), wantErr: "stream aborted", wantHashed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vm := testVMConfig()
			vm.Spec.BootDisk.Empty = false
			vm.Spec.BootDisk.Image = tt.image
			vm.Spec.BootDisk.ImageSHA256 = tt.digest
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			if tt.hashErr != nil {
				sm.volumeSHA256Func = func(context.Context, string, string) (string, error) { return "", tt.hashErr }
			}

			err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("create: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				if tt.hashErr == nil && !errors.Is(err, storage.ErrImageDigest) {
					t.Errorf("err = %v, want storage.ErrImageDigest", err)
				}
				if len(sm.createVolumeCalls) != 0 || len(lv.domainDefineXMLCalls) != 0 {
					t.Errorf("made %d volumes and %d domains for a refused image", len(sm.createVolumeCalls), len(lv.domainDefineXMLCalls))
				}
			}
			want := []string(nil)
			if tt.wantHashed {
				want = []string{storage.DefaultImagesPool + "/fedora-43.qcow2"}
			}
			if !slices.Equal(sm.volumeSHA256Calls, want) {
				t.Errorf("hashed %v, want %v", sm.volumeSHA256Calls, want)
			}
		})
	}
}
