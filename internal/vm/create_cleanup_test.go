package vm

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
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

// TestCreateFromConfigWithDeps_CleanupUndefinesWithNvram: a failed create
// undefines its domain with VIR_DOMAIN_UNDEFINE_NVRAM. A UEFI domain whose
// start got as far as making its varstore can't be undefined without it
// ("cannot undefine domain with nvram"), which left a defined domain with no
// disks.
func TestCreateFromConfigWithDeps_CleanupUndefinesWithNvram(t *testing.T) {
	lv := newMockLibvirtClient()
	lv.domainCreateFunc = func(libvirt.Domain) error { return errors.New("Cannot get interface MTU on 'br0': No such device") }
	lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateShutoff, 0, nil }
	lv.domainUndefineFunc = func(libvirt.Domain) error {
		return libvirt.Error{Code: uint32(libvirt.ErrOperationInvalid), Message: "cannot undefine domain with nvram"}
	}
	var flags []libvirt.DomainUndefineFlagsValues
	lv.domainUndefineFlagsFunc = func(_ libvirt.Domain, f libvirt.DomainUndefineFlagsValues) error {
		flags = append(flags, f)
		return nil
	}
	sm := newMockStorageManager()

	err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv))
	if err == nil {
		t.Fatal("create succeeded although the start failed")
	}
	if strings.Contains(err.Error(), "cleanup incomplete") {
		t.Errorf("cleanup reported a failure: %v", err)
	}
	if len(flags) != 1 || flags[0]&libvirt.DomainUndefineNvram == 0 || len(lv.domainUndefineCalls) != 0 {
		t.Errorf("undefine flags %v, plain undefines %d: want one undefine with VIR_DOMAIN_UNDEFINE_NVRAM", flags, len(lv.domainUndefineCalls))
	}
	if !hasCall(sm.deleteVolumeCalls, "foundry-vms/test-vm_boot.qcow2") {
		t.Errorf("boot volume not deleted: %v", sm.deleteVolumeCalls)
	}
}

// TestCreateFromConfigWithDeps_CleanupKeepsDomainItCannotStop: when the
// failed create's domain may still be running (the forced stop fails, or its
// state can't be read), the domain stays defined and its volumes stay, and
// the create's error says so. Undefining it would only make it transient,
// running on deleted disks.
func TestCreateFromConfigWithDeps_CleanupKeepsDomainItCannotStop(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(lv *mockLibvirtClient)
	}{
		{"stop fails", func(lv *mockLibvirtClient) {
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateRunning, 0, nil }
			lv.domainDestroyFunc = func(libvirt.Domain) error { return errors.New("Failed to terminate process: Device or resource busy") }
		}},
		{"state unreadable", func(lv *mockLibvirtClient) {
			lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return 0, 0, errors.New("connection reset") }
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lv := newMockLibvirtClient()
			lv.domainCreateFunc = func(libvirt.Domain) error { return errors.New("start timed out") }
			tt.setup(lv)
			sm := newMockStorageManager()

			err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv))
			if err == nil {
				t.Fatal("create succeeded although the start failed")
			}
			for _, want := range []string{"start timed out", "may still be running", "foundry-vms/test-vm_boot.qcow2", "foundry destroy test-vm"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if len(lv.domainUndefineFlagsCalls)+len(lv.domainUndefineCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
				t.Errorf("undefined %d, deleted %v: a domain that may run must keep its definition and disks",
					len(lv.domainUndefineFlagsCalls)+len(lv.domainUndefineCalls), sm.deleteVolumeCalls)
			}
		})
	}
}

// TestCreateFromConfigWithDeps_CleanupReportsDomainLeftDefined: a stopped
// domain that can't be undefined is reported in the create's error; its
// volumes, which no running guest uses, are deleted.
func TestCreateFromConfigWithDeps_CleanupReportsDomainLeftDefined(t *testing.T) {
	lv := newMockLibvirtClient()
	lv.domainCreateFunc = func(libvirt.Domain) error { return errors.New("start failed") }
	lv.domainGetStateFunc = func(libvirt.Domain, uint32) (int32, int32, error) { return domainStateShutoff, 0, nil }
	lv.domainUndefineFlagsFunc = func(libvirt.Domain, libvirt.DomainUndefineFlagsValues) error { return errors.New("undefine failed") }
	sm := newMockStorageManager()

	err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "domain 'test-vm' left defined") {
		t.Fatalf("got %v, want an error saying the domain was left defined", err)
	}
	if len(lv.domainDestroyCalls) != 0 {
		t.Errorf("stopped a domain that was shut off")
	}
	if !hasCall(sm.deleteVolumeCalls, "foundry-vms/test-vm_boot.qcow2") {
		t.Errorf("boot volume not deleted: %v", sm.deleteVolumeCalls)
	}
}

// TestCreateFromConfigWithDeps_PathImageChecked: a boot image given as a host
// path is checked as an import is, before anything is made, and its format
// comes from its content: a raw file named "disk.bin" backs the overlay as
// raw, and a qcow2 with an external data file is refused.
func TestCreateFromConfigWithDeps_PathImageChecked(t *testing.T) {
	dir := t.TempDir()
	raw := make([]byte, 1024)
	raw[510], raw[511] = 0x55, 0xaa
	rawPath := filepath.Join(dir, "disk.bin")
	if err := os.WriteFile(rawPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	q := make([]byte, 4096)
	copy(q, []byte{0x51, 0x46, 0x49, 0xfb})
	binary.BigEndian.PutUint32(q[4:], 3)   // version
	binary.BigEndian.PutUint32(q[20:], 16) // cluster_bits
	binary.BigEndian.PutUint64(q[72:], 1<<2)
	binary.BigEndian.PutUint32(q[100:], 104) // header_length
	dataFilePath := filepath.Join(dir, "disk.qcow2")
	if err := os.WriteFile(dataFilePath, q, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("raw without an extension", func(t *testing.T) {
		vm := testVMConfig()
		vm.Spec.BootDisk.Empty = false
		vm.Spec.BootDisk.Image = rawPath
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()
		if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err != nil {
			t.Fatalf("create: %v", err)
		}
		if len(sm.createVolumeCalls) == 0 || sm.createVolumeCalls[0].BackingFormat != storage.VolumeFormatRaw ||
			sm.createVolumeCalls[0].BackingVolume != rawPath {
			t.Errorf("boot volume spec %+v, want backing %s as raw", sm.createVolumeCalls, rawPath)
		}
	})
	t.Run("qcow2 with an external data file", func(t *testing.T) {
		vm := testVMConfig()
		vm.Spec.BootDisk.Empty = false
		vm.Spec.BootDisk.Image = dataFilePath
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()
		err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
		if err == nil || !strings.Contains(err.Error(), "external data file") {
			t.Fatalf("got %v, want an external data file error", err)
		}
		if len(sm.createVolumeCalls) != 0 || len(lv.domainDefineXMLCalls) != 0 {
			t.Errorf("made %v, defined %d before refusing the image", sm.createVolumeCalls, len(lv.domainDefineXMLCalls))
		}
	})
}

// TestCreateFromConfigWithDeps_Interrupted: an interrupt (Ctrl-C) between
// steps stops the create and its cleanup removes what was made so far, though
// the context has ended.
func TestCreateFromConfigWithDeps_Interrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	vm := testVMConfigWithDataDisks() // vdb, vdc
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	sm.createVolumeFunc = func(_ context.Context, _ string, spec storage.VolumeSpec) error {
		if spec.Name == "test-vm_data-vdb.qcow2" {
			cancel() // Ctrl-C while the first data disk is made
		}
		return nil
	}
	sm.deleteVolumeFunc = func(c context.Context, _, _ string) error {
		if c.Err() != nil {
			return errors.New("cleanup ran with an ended context")
		}
		return nil
	}

	err := createFromConfigWithDeps(ctx, vm, lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("got %v, want an interrupted error", err)
	}
	if strings.Contains(err.Error(), "cleanup incomplete") {
		t.Errorf("cleanup failed: %v", err)
	}
	if got := strings.Join(sm.deleteVolumeCalls, " "); got != "foundry-vms/test-vm_boot.qcow2 foundry-vms/test-vm_data-vdb.qcow2" {
		t.Errorf("deleted %q, want the boot and first data volume", got)
	}
	if len(lv.domainDefineXMLCalls) != 0 {
		t.Error("defined the domain after the interrupt")
	}
}
