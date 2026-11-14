package vm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/digitalocean/go-libvirt"
	"github.com/kdomanski/iso9660"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

// testInputDisks returns two input disks: NODECFG with a file read from dir
// and an inline file, and EXTRA with one inline file.
func testInputDisks(t *testing.T, dir string) []v1alpha1.InputDiskSpec {
	t.Helper()
	path := filepath.Join(dir, "node.yaml")
	if err := os.WriteFile(path, []byte("node: one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []v1alpha1.InputDiskSpec{
		{Label: "NODECFG", Files: []v1alpha1.InputFileSpec{
			{Name: "node.yaml", File: path},
			{Name: "install-config", String: "disk=/dev/vda\n"},
		}},
		{Label: "EXTRA", Files: []v1alpha1.InputFileSpec{{Name: "a.txt", String: "a"}}},
	}
}

// isoFiles opens an ISO image and returns its label and root files by name.
func isoFiles(t *testing.T, data []byte) (string, map[string]string) {
	t.Helper()
	img, err := iso9660.OpenImage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open ISO: %v", err)
	}
	label, err := img.Label()
	if err != nil {
		t.Fatal(err)
	}
	root, err := img.RootDir()
	if err != nil {
		t.Fatal(err)
	}
	children, err := root.GetChildren()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, c := range children {
		b, err := io.ReadAll(c.Reader())
		if err != nil {
			t.Fatal(err)
		}
		files[c.Name()] = string(b)
	}
	return label, files
}

func TestBuildInputImages(t *testing.T) {
	vm := testVMConfig()
	vm.Spec.InputDisks = testInputDisks(t, t.TempDir())

	images, err := buildInputImages(vm)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].volume != "test-vm_input-nodecfg.iso" || images[1].volume != "test-vm_input-extra.iso" {
		t.Fatalf("images %+v", images)
	}
	label, files := isoFiles(t, images[0].data)
	if label != "NODECFG" {
		t.Errorf("label %q", label)
	}
	if files["node.yaml"] != "node: one\n" || files["install-config"] != "disk=/dev/vda\n" || len(files) != 2 {
		t.Errorf("files %q", files)
	}
	if len(images[0].data)%2048 != 0 {
		t.Errorf("image size %d is not whole ISO-9660 sectors", len(images[0].data))
	}
}

// The longest label and file name validation allows reach the guest
// unchanged, and an empty file is fine.
func TestBuildInputImages_Limits(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	label := strings.Repeat("L", 32)
	name := strings.Repeat("n", 19) + ".abcdefgh" // 28 characters
	vm := testVMConfig()
	vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: label, Files: []v1alpha1.InputFileSpec{
		{Name: name, String: "x"},
		{Name: "empty", File: empty},
	}}}
	images, err := buildInputImages(vm)
	if err != nil {
		t.Fatal(err)
	}
	gotLabel, files := isoFiles(t, images[0].data)
	if gotLabel != label {
		t.Errorf("label %q, want %q", gotLabel, label)
	}
	if v, ok := files[name]; !ok || v != "x" {
		t.Errorf("files %q lack %s", files, name)
	}
	if v, ok := files["empty"]; !ok || v != "" {
		t.Errorf("files %q lack the empty file", files)
	}
}

func TestBuildInputImages_Errors(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, make([]byte, maxInputDiskBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	half := strings.Repeat("x", maxInputDiskBytes/2+1)

	tests := []struct {
		name  string
		files []v1alpha1.InputFileSpec
		want  string
	}{
		{"missing file", []v1alpha1.InputFileSpec{{Name: "a", File: filepath.Join(dir, "missing")}}, "spec.inputDisks[0].files[0]"},
		{"directory", []v1alpha1.InputFileSpec{{Name: "a", File: dir}}, "not a regular file"},
		// A device that never ends must not be read to the end
		{"endless device", []v1alpha1.InputFileSpec{{Name: "a", File: "/dev/zero"}}, "not a regular file"},
		// Opening a FIFO must not wait for a writer
		{"fifo", []v1alpha1.InputFileSpec{{Name: "a", File: fifo}}, "not a regular file"},
		{"oversized file", []v1alpha1.InputFileSpec{{Name: "a", File: big}}, "larger than 16 MiB"},
		{"oversized disk", []v1alpha1.InputFileSpec{{Name: "a", String: half}, {Name: "b", String: half}}, "larger than 16 MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := testVMConfig()
			vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: "X", Files: tt.files}}
			if _, err := buildInputImages(vm); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCreateFromConfigWithDeps_InputDisks(t *testing.T) {
	vm := testVMConfigWithCloudInit()
	vm.Spec.InputDisks = testInputDisks(t, t.TempDir())
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()

	if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err != nil {
		t.Fatal(err)
	}
	var input []storage.VolumeSpec
	for _, spec := range sm.createVolumeCalls {
		if spec.Type == storage.VolumeTypeInput {
			input = append(input, spec)
		}
	}
	if len(input) != 2 || input[0].Name != "test-vm_input-nodecfg.iso" || input[0].Format != storage.VolumeFormatRaw ||
		input[0].CapacityBytes == 0 || input[0].CapacityBytes%2048 != 0 {
		t.Errorf("input volumes %+v", input)
	}
	for _, v := range []string{"foundry-vms/test-vm_input-nodecfg.iso", "foundry-vms/test-vm_input-extra.iso"} {
		if !containsString(sm.writeVolumeDataCalls, v) {
			t.Errorf("%s not written: %v", v, sm.writeVolumeDataCalls)
		}
		if !containsString(sm.volumeExistsCalls, v) {
			t.Errorf("%s not checked before create: %v", v, sm.volumeExistsCalls)
		}
	}
	defined := lv.domainDefineXMLCalls[0]
	for _, want := range []string{`volume="test-vm_input-nodecfg.iso"`, `<target dev="sdb" bus="sata">`, `<target dev="sdc" bus="sata">`} {
		if !strings.Contains(defined, want) {
			t.Errorf("domain XML lacks %s", want)
		}
	}
}

// A missing input file fails before any storage is created.
func TestCreateFromConfigWithDeps_InputFileMissing(t *testing.T) {
	vm := testVMConfig()
	vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: "X", Files: []v1alpha1.InputFileSpec{
		{Name: "a", File: filepath.Join(t.TempDir(), "missing")},
	}}}
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err == nil {
		t.Fatal("expected an error")
	}
	if len(sm.createVolumeCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
		t.Errorf("volumes created %+v, deleted %v", sm.createVolumeCalls, sm.deleteVolumeCalls)
	}
}

// An input volume left over from an earlier VM of the same name fails the
// create before any storage is created, and is neither reused nor deleted.
func TestCreateFromConfigWithDeps_InputVolumeExists(t *testing.T) {
	vm := testVMConfig()
	vm.Spec.InputDisks = testInputDisks(t, t.TempDir())
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	sm.volumeExistsFunc = func(ctx context.Context, poolName, volumeName string) (bool, error) {
		return volumeName == "test-vm_input-extra.iso", nil
	}
	err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "input volume already exists: foundry-vms/test-vm_input-extra.iso") {
		t.Fatalf("error %v", err)
	}
	if len(sm.createVolumeCalls) != 0 || len(sm.deleteVolumeCalls) != 0 {
		t.Errorf("volumes created %+v, deleted %v", sm.createVolumeCalls, sm.deleteVolumeCalls)
	}
}

// A failed upload cleans up the input volumes with the rest.
func TestCreateFromConfigWithDeps_InputUploadFails(t *testing.T) {
	vm := testVMConfig()
	vm.Spec.InputDisks = testInputDisks(t, t.TempDir())
	lv := newMockLibvirtClient()
	sm := newMockStorageManager()
	sm.writeVolumeDataFunc = func(ctx context.Context, poolName, volumeName string, data []byte) error {
		if volumeName == "test-vm_input-extra.iso" {
			return errors.New("upload failed")
		}
		return nil
	}
	err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
	if err == nil || !strings.Contains(err.Error(), "failed to write input volume test-vm_input-extra.iso") {
		t.Fatalf("error %v", err)
	}
	want := "foundry-vms/test-vm_boot.qcow2,foundry-vms/test-vm_input-nodecfg.iso,foundry-vms/test-vm_input-extra.iso"
	if got := strings.Join(sm.deleteVolumeCalls, ","); got != want {
		t.Errorf("deleted %s, want %s", got, want)
	}
}

// A domain that fails to start takes its input volumes with it.
func TestCreateFromConfigWithDeps_InputDisksStartFails(t *testing.T) {
	vm := testVMConfig()
	vm.Spec.InputDisks = testInputDisks(t, t.TempDir())
	lv := newMockLibvirtClient()
	lv.domainCreateFunc = func(dom libvirt.Domain) error { return errors.New("start failed") }
	sm := newMockStorageManager()
	if err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv)); err == nil {
		t.Fatal("expected an error")
	}
	for _, v := range []string{"foundry-vms/test-vm_input-nodecfg.iso", "foundry-vms/test-vm_input-extra.iso"} {
		if !containsString(sm.deleteVolumeCalls, v) {
			t.Errorf("%s not deleted: %v", v, sm.deleteVolumeCalls)
		}
	}
	if len(lv.domainUndefineFlagsCalls) != 1 {
		t.Errorf("domain undefined %d times", len(lv.domainUndefineFlagsCalls))
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
