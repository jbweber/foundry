package vm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/metadata"
	"github.com/jbweber/foundry/internal/storage"
)

// withSpec makes the mock's domains carry foundry's stored spec for vm, as
// metadata.Client stores it; any other metadata namespace has none.
func withSpec(t *testing.T, lv *mockLibvirtClient, vm *v1alpha1.VirtualMachine) {
	t.Helper()
	var stored string
	lv.domainSetMetadataFunc = func(_ libvirt.Domain, _ int32, value libvirt.OptString, _ libvirt.OptString, _ libvirt.OptString, _ libvirt.DomainModificationImpact) error {
		stored = value[0]
		return nil
	}
	if err := metadata.NewClient(lv).Store(libvirt.Domain{Name: vm.Name}, vm); err != nil {
		t.Fatalf("store spec: %v", err)
	}
	lv.domainGetMetadataFunc = func(_ libvirt.Domain, _ int32, uri libvirt.OptString, _ libvirt.DomainModificationImpact) (string, error) {
		if len(uri) == 1 && uri[0] == metadata.MetadataNamespace {
			return stored, nil
		}
		return "", libvirt.Error{Code: uint32(libvirt.ErrNoDomainMetadata)}
	}
}

// noSpec makes the mock's domains carry no foundry metadata, as a domain
// made by other means (virt-install, another tool) has none.
func noSpec(lv *mockLibvirtClient) {
	lv.domainGetMetadataFunc = func(libvirt.Domain, int32, libvirt.OptString, libvirt.DomainModificationImpact) (string, error) {
		return "", libvirt.Error{Code: uint32(libvirt.ErrNoDomainMetadata)}
	}
}

// poolMocks returns mocks with a stopped domain of any name and the given
// volumes, each "pool/name".
func poolMocks(volumes ...string) (*mockLibvirtClient, *mockStorageManager) {
	lv := newMockLibvirtClient()
	lv.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) { return libvirt.Domain{Name: name}, nil }
	lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateShutoff, 0, nil }
	sm := newMockStorageManager()
	sm.listVolumesFunc = func(_ context.Context, pool string) ([]storage.VolumeInfo, error) {
		var out []storage.VolumeInfo
		for _, v := range volumes {
			if p, name, _ := strings.Cut(v, "/"); p == pool {
				out = append(out, storage.VolumeInfo{Name: name, Pool: pool})
			}
		}
		return out, nil
	}
	return lv, sm
}

func deleted(sm *mockStorageManager) string {
	got := append([]string(nil), sm.deleteVolumeCalls...)
	sort.Strings(got)
	return strings.Join(got, " ")
}

// imagesPoolVolumes are images-pool volumes that belong to no VM, though
// their names start with "rhel_": base images and an installer ISO, next to
// a Secure Boot key profile and another image. No destroy of a VM named "rhel" may
// delete them.
var imagesPoolVolumes = []string{
	"foundry-images/rhel_10.qcow2",
	"foundry-images/rhel_10.2-x86_64.iso",
	"foundry-images/rhel_base.qcow2",
	"foundry-images/varstore_corp-keys_x86_64.fd",
	"foundry-images/fedora.qcow2",
}

// TestDestroyWithDeps_KeepsSamePrefixImages: destroying a VM in the default
// pool deletes its volumes there and nothing from the images pool, however
// the images are named, even one with a VM volume's name.
func TestDestroyWithDeps_KeepsSamePrefixImages(t *testing.T) {
	lv, sm := poolMocks(append([]string{
		"foundry-vms/rhel_boot.qcow2",
		"foundry-vms/rhel_cloudinit.iso",
		"foundry-images/rhel_boot.qcow2", // an image with a VM volume's name
	}, imagesPoolVolumes...)...)
	withSpec(t, lv, &v1alpha1.VirtualMachine{ObjectMeta: v1alpha1.ObjectMeta{Name: "rhel"}})

	if err := destroyWithDeps(context.Background(), "rhel", lv, sm, false); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if got, want := deleted(sm), "foundry-vms/rhel_boot.qcow2 foundry-vms/rhel_cloudinit.iso"; got != want {
		t.Errorf("deleted %s, want %s", got, want)
	}
	for _, pool := range sm.listVolumesCalls {
		if pool == storage.DefaultImagesPool {
			t.Errorf("the images pool was searched for a VM in %s", storage.DefaultVMsPool)
		}
	}
}

// TestDestroyWithDeps_OwnPoolIsImagesPool: a VM whose spec puts its disks
// in the images pool loses exactly its own volumes there (boot, cloud-init,
// its spec's data and input disks); images named like it stay.
func TestDestroyWithDeps_OwnPoolIsImagesPool(t *testing.T) {
	own := []string{
		"foundry-images/rhel_boot.qcow2",
		"foundry-images/rhel_cloudinit.iso",
		"foundry-images/rhel_data-vdb.qcow2",
		"foundry-images/rhel_input-nodecfg.iso",
	}
	keep := []string{
		"foundry-images/rhel_data-vdc.qcow2",    // no such data disk in the spec
		"foundry-images/rhel_input-other.iso",   // no such input disk in the spec
		"foundry-images/rhel_boot.qcow2.backup", // not the name itself
	}
	lv, sm := poolMocks(append(append(own, keep...), imagesPoolVolumes...)...)
	withSpec(t, lv, &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "rhel"},
		Spec: v1alpha1.VirtualMachineSpec{
			StoragePool: storage.DefaultImagesPool,
			DataDisks:   []v1alpha1.DataDiskSpec{{Device: "vdb"}},
			InputDisks:  []v1alpha1.InputDiskSpec{{Label: "NODECFG"}},
		},
	})

	if err := destroyWithDeps(context.Background(), "rhel", lv, sm, false); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	want := append([]string(nil), own...)
	sort.Strings(want)
	if got := deleted(sm); got != strings.Join(want, " ") {
		t.Errorf("deleted %s,\nwant    %s", got, strings.Join(want, " "))
	}
}

// TestDestroyWithDeps_KeepsOtherVMsVolumes: destroying "web" leaves the
// volumes of a defined VM "web_2", though they start with "web_".
func TestDestroyWithDeps_KeepsOtherVMsVolumes(t *testing.T) {
	lv, sm := poolMocks("foundry-vms/web_boot.qcow2", "foundry-vms/web_2_boot.qcow2")
	lv.connectListAllDomainsFunc = func(int32, libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
		return []libvirt.Domain{{Name: "web"}, {Name: "web_2"}}, 2, nil
	}
	withSpec(t, lv, &v1alpha1.VirtualMachine{ObjectMeta: v1alpha1.ObjectMeta{Name: "web"}})

	if err := destroyWithDeps(context.Background(), "web", lv, sm, false); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if got := deleted(sm); got != "foundry-vms/web_boot.qcow2" {
		t.Errorf("deleted %s, want only foundry-vms/web_boot.qcow2", got)
	}
}

// TestDestroyWithDeps_ListingFails: when the domains can't be listed,
// destroy can't tell another VM's "web_..." volumes from its own, so it
// stops before it changes anything, also when forced: not stopped, not
// undefined, no volume deleted.
func TestDestroyWithDeps_ListingFails(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			lv, sm := poolMocks("foundry-vms/web_boot.qcow2", "foundry-vms/web_2_boot.qcow2", "foundry-vms/web_2_data-vdb.qcow2")
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateRunning, 0, nil }
			lv.connectListAllDomainsFunc = func(int32, libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error) {
				return nil, 0, fmt.Errorf("connection reset")
			}
			if force {
				noSpec(lv)
			} else {
				withSpec(t, lv, &v1alpha1.VirtualMachine{ObjectMeta: v1alpha1.ObjectMeta{Name: "web"}})
			}

			err := destroyWithDeps(context.Background(), "web", lv, sm, force)
			if err == nil || !strings.Contains(err.Error(), "cannot list the domains") || !strings.Contains(err.Error(), "nothing changed") {
				t.Fatalf("got %v, want an error saying the domains cannot be listed and nothing changed", err)
			}
			if len(lv.domainShutdownCalls) != 0 || len(lv.domainDestroyCalls) != 0 ||
				len(lv.domainUndefineFlagsCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
				t.Errorf("changed something although the domains could not be listed: shutdowns %d, destroys %d, undefines %d, deleted %s",
					len(lv.domainShutdownCalls), len(lv.domainDestroyCalls), len(lv.domainUndefineFlagsCalls), deleted(sm))
			}
		})
	}
}

// TestDestroyWithDeps_RefusesForeignDomain: a domain without foundry's spec,
// or with one that cannot be read, is refused with a clear error and
// nothing changed: not stopped, not undefined, no volume deleted.
func TestDestroyWithDeps_RefusesForeignDomain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*mockLibvirtClient)
		want  string
	}{
		{"no spec", noSpec, "VM 'rhel' is not foundry's (its domain has no foundry spec); nothing changed, use --force to destroy it anyway"},
		{"unreadable spec", func(lv *mockLibvirtClient) {
			lv.domainGetMetadataFunc = func(libvirt.Domain, int32, libvirt.OptString, libvirt.DomainModificationImpact) (string, error) {
				return "<metadata", nil
			}
		}, "VM 'rhel': cannot read its foundry spec, nothing changed (use --force to destroy it anyway)"},
		{"metadata error", func(lv *mockLibvirtClient) {
			lv.domainGetMetadataFunc = func(libvirt.Domain, int32, libvirt.OptString, libvirt.DomainModificationImpact) (string, error) {
				return "", fmt.Errorf("connection reset")
			}
		}, "cannot read its foundry spec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lv, sm := poolMocks("foundry-vms/rhel_boot.qcow2")
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateRunning, 0, nil }
			tc.setup(lv)

			err := destroyWithDeps(context.Background(), "rhel", lv, sm, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
			if len(lv.domainGetStateCalls) != 0 || len(lv.domainShutdownCalls) != 0 || len(lv.domainDestroyCalls) != 0 ||
				len(lv.domainUndefineFlagsCalls) != 0 || len(sm.listVolumesCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
				t.Errorf("nothing may change for a refused domain")
			}
		})
	}
}

// TestDestroyWithDeps_ForceForeignDomain: --force destroys a domain without
// foundry's spec, searching the VMs pool only.
func TestDestroyWithDeps_ForceForeignDomain(t *testing.T) {
	lv, sm := poolMocks(append([]string{"foundry-vms/rhel_boot.qcow2"}, imagesPoolVolumes...)...)
	noSpec(lv)

	if err := destroyWithDeps(context.Background(), "rhel", lv, sm, true); err != nil {
		t.Fatalf("forced destroy failed: %v", err)
	}
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("expected the domain undefined, got %d undefine calls", len(lv.domainUndefineFlagsCalls))
	}
	if got := deleted(sm); got != "foundry-vms/rhel_boot.qcow2" {
		t.Errorf("deleted %s, want only foundry-vms/rhel_boot.qcow2", got)
	}
}
