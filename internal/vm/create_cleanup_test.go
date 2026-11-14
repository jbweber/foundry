package vm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

// volumeExistsError is libvirt's answer when a volume of that name exists.
var volumeExistsError = libvirt.Error{Code: uint32(libvirt.ErrStorageVolExist), Message: "storage volume already exists"}

func hasCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

// TestCreateFromConfigWithDeps_CleanupDeletesOnlyVolumesItMade: when making a
// volume fails because one of that name exists (made by something else since
// the pre-flight check), the failed create deletes the volumes it made and
// not that one, nor any volume the spec names that it never got to.
func TestCreateFromConfigWithDeps_CleanupDeletesOnlyVolumesItMade(t *testing.T) {
	tests := []struct {
		name    string
		vm      func() *v1alpha1.VirtualMachine
		taken   string // pool/name of the volume that exists
		want    []string
		notMade []string
	}{
		{
			name: "data volume in the images pool",
			vm: func() *v1alpha1.VirtualMachine {
				vm := testVMConfigWithDataDisks() // vdb, vdc
				vm.Spec.StoragePool = storage.DefaultImagesPool
				return vm
			},
			taken:   storage.DefaultImagesPool + "/test-vm_data-vdb.qcow2",
			want:    []string{storage.DefaultImagesPool + "/test-vm_boot.qcow2"},
			notMade: []string{storage.DefaultImagesPool + "/test-vm_data-vdc.qcow2"},
		},
		{
			name:  "cloud-init volume",
			vm:    testVMConfigWithCloudInit,
			taken: "foundry-vms/test-vm_cloudinit.iso",
			want:  []string{"foundry-vms/test-vm_boot.qcow2"},
		},
		{
			// VM "a" with data disk "vdb_boot" names VM "a_data-vdb"'s boot
			// volume; the loader now refuses both names, create's cleanup
			// must not depend on that
			name: "data volume named like another VM's boot volume",
			vm: func() *v1alpha1.VirtualMachine {
				vm := testVMConfig()
				vm.Name = "a"
				vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{{Device: "vdb_boot", SizeGB: 1}}
				return vm
			},
			taken: "foundry-vms/a_data-vdb_boot.qcow2",
			want:  []string{"foundry-vms/a_boot.qcow2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := tt.vm()
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			sm.createVolumeFunc = func(_ context.Context, pool string, spec storage.VolumeSpec) error {
				if pool+"/"+spec.Name == tt.taken {
					return volumeExistsError
				}
				return nil
			}

			if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err == nil {
				t.Fatal("create succeeded although a volume of its name exists")
			}
			if hasCall(sm.deleteVolumeCalls, tt.taken) {
				t.Errorf("failed create deleted %s, which it did not make; deletes: %v", tt.taken, sm.deleteVolumeCalls)
			}
			for _, v := range tt.notMade {
				if hasCall(sm.deleteVolumeCalls, v) {
					t.Errorf("failed create deleted %s, which it never made; deletes: %v", v, sm.deleteVolumeCalls)
				}
			}
			if strings.Join(sm.deleteVolumeCalls, " ") != strings.Join(tt.want, " ") {
				t.Errorf("deletes %v, want %v", sm.deleteVolumeCalls, tt.want)
			}
		})
	}
}

// TestCreateFromConfigWithDeps_CleanupDeletesEveryVolumeItMade: a failure
// after all volumes are made deletes each of them.
func TestCreateFromConfigWithDeps_CleanupDeletesEveryVolumeItMade(t *testing.T) {
	vm := testVMConfigWithCloudInit()
	vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{{Device: "vdb", SizeGB: 1}}
	vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: "CFG", Files: []v1alpha1.InputFileSpec{{Name: "a", String: "x"}}}}
	lv := newMockLibvirtClient()
	lv.domainDefineXMLFunc = func(string) (libvirt.Domain, error) { return libvirt.Domain{}, errors.New("define failed") }
	sm := newMockStorageManager()

	if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err == nil {
		t.Fatal("create succeeded although the define failed")
	}
	want := "foundry-vms/test-vm_boot.qcow2 foundry-vms/test-vm_data-vdb.qcow2 foundry-vms/test-vm_cloudinit.iso foundry-vms/test-vm_input-cfg.iso"
	if got := strings.Join(sm.deleteVolumeCalls, " "); got != want {
		t.Errorf("deletes %q, want %q", got, want)
	}
}

// TestCreateFromConfigWithDeps_RefusesExistingVolume: create checks that none
// of the VM's volumes exists before it makes anything, and names the one
// that does.
func TestCreateFromConfigWithDeps_RefusesExistingVolume(t *testing.T) {
	for _, tt := range []struct{ existing, want string }{
		{"test-vm_boot.qcow2", "boot volume already exists: foundry-vms/test-vm_boot.qcow2"},
		{"test-vm_data-vdb.qcow2", "data volume already exists: foundry-vms/test-vm_data-vdb.qcow2"},
		{"test-vm_cloudinit.iso", "cloud-init volume already exists: foundry-vms/test-vm_cloudinit.iso"},
		{"test-vm_input-cfg.iso", "input volume already exists: foundry-vms/test-vm_input-cfg.iso"},
	} {
		t.Run(tt.existing, func(t *testing.T) {
			vm := testVMConfigWithCloudInit()
			vm.Spec.DataDisks = []v1alpha1.DataDiskSpec{{Device: "vdb", SizeGB: 1}}
			vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: "CFG", Files: []v1alpha1.InputFileSpec{{Name: "a", String: "x"}}}}
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			sm.volumeExistsFunc = func(_ context.Context, pool, name string) (bool, error) {
				return pool == "foundry-vms" && name == tt.existing, nil
			}

			err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
			if len(sm.createVolumeCalls) != 0 || len(sm.deleteVolumeCalls) != 0 || len(lv.domainDefineXMLCalls) != 0 {
				t.Errorf("made %v, deleted %v, defined %d before refusing", sm.createVolumeCalls, sm.deleteVolumeCalls, len(lv.domainDefineXMLCalls))
			}
		})
	}
}
