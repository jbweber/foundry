package vm

import (
	"context"
	"strings"
	"testing"
)

// TestCreateFromConfigWithDeps_RefusesOtherImagePool: create takes a boot
// image from foundry-images only, so a spec naming another pool (one that
// did not come through the loader, which refuses it too) is refused before
// anything is made, rather than booting foundry-images' volume of that name.
func TestCreateFromConfigWithDeps_RefusesOtherImagePool(t *testing.T) {
	for _, set := range []func(image, pool *string){
		func(image, _ *string) { *image = "other-pool:fedora.qcow2" },
		func(image, pool *string) { *image = "fedora.qcow2"; *pool = "other-pool" },
	} {
		vm := testVMConfig()
		vm.Spec.BootDisk.Empty = false
		set(&vm.Spec.BootDisk.Image, &vm.Spec.BootDisk.ImagePool)
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()

		err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
		if err == nil || !strings.Contains(err.Error(), `image pool "other-pool"`) {
			t.Fatalf("image %q pool %q: got %v, want an image pool error", vm.Spec.BootDisk.Image, vm.Spec.BootDisk.ImagePool, err)
		}
		if len(sm.createVolumeCalls) != 0 || len(sm.imageExistsCalls) != 0 || len(lv.domainDefineXMLCalls) != 0 {
			t.Errorf("made %v, looked up %v, defined %d before refusing", sm.createVolumeCalls, sm.imageExistsCalls, len(lv.domainDefineXMLCalls))
		}
	}
}
