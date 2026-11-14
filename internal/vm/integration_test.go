//go:build integration

// Integration tests for foundry against a real libvirt, black-box: they use
// only what the packages export, as the CLI does. They are built only with
// the integration tag (make test-integration) and run only when
// FOUNDRY_INTEGRATION_SOCKET names the socket of a libvirt to test against.
// That libvirt should be a disposable one, such as a throwaway VM's: the
// tests create and delete their own VMs, a network and images there, and
// foundry's default pools when they are missing, and touch nothing else, but
// a test machine is the place for them, never one whose VMs matter.
//
// Environment:
//   - FOUNDRY_INTEGRATION_SOCKET: the libvirt socket (local, or one forwarded
//     from a libvirt on another machine). Required; without it every test is
//     skipped, and make test-integration fails because nothing ran.
//
// The tests that start a VM need KVM on that libvirt's machine: a VM without
// spec.arch runs under KVM. They fail, saying so, when libvirt offers none.
package vm_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/digitalocean/go-libvirt"
	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
	"github.com/jbweber/foundry/internal/storage"
	"github.com/jbweber/foundry/internal/vm"
)

const (
	envSocket = "FOUNDRY_INTEGRATION_SOCKET"

	// namePrefix starts the name of everything the tests create, so what
	// they made can be told from anything else on the machine.
	namePrefix = "foundry-it-"

	callTimeout = 2 * time.Minute
)

// integrationSocket returns the socket to test against, or skips the test,
// naming what is missing.
func integrationSocket(t *testing.T) string {
	t.Helper()
	socket := os.Getenv(envSocket)
	if socket == "" {
		t.Skipf("libvirt: %s is not set; set it to the socket of a disposable libvirt to run this test", envSocket)
	}
	if _, err := os.Stat(socket); err != nil {
		t.Skipf("libvirt: no socket at %s=%s: %v", envSocket, socket, err)
	}
	return socket
}

func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	t.Cleanup(cancel)
	return ctx
}

// setup points foundry at the test libvirt and returns a connection for
// what the tests set up and check beside foundry (a network, the pools), and
// a storage manager on it. foundry's default pools that do not exist yet are
// created here; they are removed again when the test ends, with whatever the
// test left in them, so a libvirt without them is left without them.
func setup(t *testing.T) (*libvirt.Libvirt, *storage.Manager) {
	t.Helper()
	socket := integrationSocket(t)
	foundrylibvirt.SetSocket(socket)
	t.Cleanup(func() { foundrylibvirt.SetSocket("") })

	c, err := foundrylibvirt.ConnectWithContext(ctxT(t), socket, 0)
	if err != nil {
		t.Fatalf("connect to %s: %v", socket, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	lv := c.Libvirt()

	var missing []string
	for _, p := range []string{storage.DefaultImagesPool, storage.DefaultVMsPool, storage.DefaultLogsPool} {
		if _, err := lv.StoragePoolLookupByName(p); err != nil {
			if !isLibvirtError(err, libvirt.ErrNoStoragePool) {
				t.Fatalf("look up pool %s: %v", p, err)
			}
			missing = append(missing, p)
		}
	}
	// Registered before the pools are made, so a half-made set is removed too
	t.Cleanup(func() {
		for _, p := range missing {
			removePool(t, lv, p)
		}
	})
	mgr := storage.NewManager(lv)
	if err := mgr.EnsureDefaultPools(ctxT(t)); err != nil {
		t.Fatalf("make the default pools: %v", err)
	}
	return lv, mgr
}

func isLibvirtError(err error, code libvirt.ErrorNumber) bool {
	var e libvirt.Error
	return errors.As(err, &e) && e.Code == uint32(code)
}

// removePool removes a pool the test created: its volumes, its directory
// and its definition.
func removePool(t *testing.T, lv *libvirt.Libvirt, name string) {
	t.Helper()
	pool, err := lv.StoragePoolLookupByName(name)
	if err != nil {
		if !isLibvirtError(err, libvirt.ErrNoStoragePool) {
			t.Errorf("clean-up: look up pool %s: %v", name, err)
		}
		return
	}
	vols, _, err := lv.StoragePoolListAllVolumes(pool, 1, 0)
	if err != nil {
		t.Errorf("clean-up: list volumes of pool %s: %v", name, err)
	}
	for _, v := range vols {
		if err := lv.StorageVolDelete(v, 0); err != nil {
			t.Errorf("clean-up: delete volume %s/%s: %v", name, v.Name, err)
		}
	}
	if err := lv.StoragePoolDestroy(pool); err != nil {
		t.Errorf("clean-up: stop pool %s: %v", name, err)
	}
	if err := lv.StoragePoolDelete(pool, 0); err != nil {
		t.Errorf("clean-up: delete the directory of pool %s: %v", name, err)
	}
	if err := lv.StoragePoolUndefine(pool); err != nil {
		t.Errorf("clean-up: undefine pool %s: %v", name, err)
	}
}

// uniqueName returns namePrefix plus a random suffix.
func uniqueName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return namePrefix + hex.EncodeToString(b)
}

// shortName is a name for a Linux interface (at most 15 characters) made
// from one of uniqueName's.
func shortName(prefix, name string) string {
	return prefix + strings.TrimPrefix(name, namePrefix)
}

// isolatedNetwork creates a transient libvirt network with no uplink and no
// addresses, and returns the name of its bridge for the test VM to attach
// to. It is destroyed when the test ends; being transient, nothing of it
// outlives a libvirt restart either.
func isolatedNetwork(t *testing.T, lv *libvirt.Libvirt, name string) string {
	t.Helper()
	bridge := shortName("fdryit", name)
	xml := fmt.Sprintf("<network><name>%s</name><bridge name='%s'/></network>", name, bridge)
	net, err := lv.NetworkCreateXML(xml)
	if err != nil {
		t.Fatalf("create network %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := lv.NetworkDestroy(net); err != nil {
			t.Errorf("clean-up: destroy network %s: %v", name, err)
		}
	})
	return bridge
}

// requireKVM fails the test when the libvirt offers no KVM for its host's
// architecture: foundry starts a VM without spec.arch under KVM.
func requireKVM(t *testing.T, lv *libvirt.Libvirt) {
	t.Helper()
	xml, err := lv.ConnectGetCapabilities()
	if err != nil {
		t.Fatalf("read capabilities: %v", err)
	}
	var caps libvirtxml.Caps
	if err := caps.Unmarshal(xml); err != nil {
		t.Fatalf("parse capabilities: %v", err)
	}
	for _, g := range caps.Guests {
		if g.OSType != "hvm" || caps.Host.CPU == nil || g.Arch.Name != caps.Host.CPU.Arch {
			continue
		}
		for _, d := range g.Arch.Domains {
			if d.Type == "kvm" {
				return
			}
		}
	}
	t.Fatal("libvirt offers no KVM on its host (no /dev/kvm, or nested virtualization is off): this test starts a VM under KVM")
}

// rawImage is a 1 MiB raw disk image that passes foundry's import check (a
// boot sector signature) and boots nothing.
func rawImage() []byte {
	img := make([]byte, 1<<20)
	img[510], img[511] = 0x55, 0xaa
	return img
}

// qcow2Header is size bytes that start with a qcow2 version 3 header: enough
// for the import's checks and libvirt's probe, not for a guest. backing, when
// not zero, is its backing_file_offset.
func qcow2Header(size int, backing uint64) []byte {
	b := make([]byte, size)
	copy(b, "QFI\xfb")
	binary.BigEndian.PutUint32(b[4:], 3)
	binary.BigEndian.PutUint64(b[8:], backing)
	binary.BigEndian.PutUint32(b[20:], 16)
	binary.BigEndian.PutUint64(b[24:], 1<<30)
	binary.BigEndian.PutUint32(b[100:], 104)
	return b
}

// digestOf is the sha256 of content in lower-case hex.
func digestOf(content []byte) string {
	d := sha256.Sum256(content)
	return hex.EncodeToString(d[:])
}

// importImage imports content as an image and deletes it, if it is there,
// when the test ends.
func importImage(t *testing.T, mgr *storage.Manager, name string, content []byte) error {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if ok, err := mgr.ImageExists(ctx, name); err != nil || !ok {
			return
		}
		if err := mgr.DeleteImage(ctx, name); err != nil {
			t.Errorf("clean-up: delete image %s: %v", name, err)
		}
	})
	return mgr.ImportImageFromReader(ctxT(t), bytes.NewReader(content), name)
}

func imageExists(t *testing.T, mgr *storage.Manager, name string) bool {
	t.Helper()
	ok, err := mgr.ImageExists(ctxT(t), name)
	if err != nil {
		t.Fatalf("look up image %s: %v", name, err)
	}
	return ok
}

// vmVolumes lists the volumes of the VM name in foundry-vms.
func vmVolumes(t *testing.T, mgr *storage.Manager, name string) []string {
	t.Helper()
	vols, err := mgr.ListVolumes(ctxT(t), storage.DefaultVMsPool)
	if err != nil {
		t.Fatalf("list volumes of %s: %v", storage.DefaultVMsPool, err)
	}
	var mine []string
	for _, v := range vols {
		if strings.HasPrefix(v.Name, name+"_") {
			mine = append(mine, v.Name)
		}
	}
	return mine
}

// destroyAtEnd destroys the VM name when the test ends, unless *done.
func destroyAtEnd(t *testing.T, lv *libvirt.Libvirt, name string, done *bool) {
	t.Helper()
	t.Cleanup(func() {
		if *done {
			return
		}
		if _, err := lv.DomainLookupByName(name); libvirt.IsNotFound(err) {
			return
		}
		if err := vm.Destroy(context.Background(), name, false); err != nil {
			t.Errorf("clean-up: destroy VM %s: %v", name, err)
		}
	})
}

func TestIntegrationConnect(t *testing.T) {
	lv, _ := setup(t)
	version, err := lv.ConnectGetLibVersion()
	if err != nil || version == 0 {
		t.Fatalf("ConnectGetLibVersion = %d, %v", version, err)
	}
	arch, err := foundrylibvirt.HostArch(lv)
	if err != nil {
		t.Fatalf("HostArch: %v", err)
	}
	t.Logf("libvirt %d.%d.%d on %s", version/1000000, version/1000%1000, version%1000, arch)
}

// TestIntegrationImportImage: each format reads back as its source's digest
// (a raw volume is exactly its image's size), so a create can check it; a
// name in the pool is refused and the image there is left; a qcow2 with a
// backing file and a reader that fails leave no volume.
func TestIntegrationImportImage(t *testing.T) {
	_, mgr := setup(t)
	for ext, content := range map[string][]byte{"raw": rawImage(), "qcow2": qcow2Header(2<<20, 0)} {
		t.Run(ext, func(t *testing.T) {
			name := uniqueName(t) + "." + ext
			if err := importImage(t, mgr, name, content); err != nil {
				t.Fatalf("import: %v", err)
			}
			got, err := mgr.VolumeSHA256(ctxT(t), storage.DefaultImagesPool, name)
			if err != nil {
				t.Fatalf("VolumeSHA256: %v", err)
			}
			if got != digestOf(content) {
				t.Errorf("volume reads back as %s, the image is %s", got, digestOf(content))
			}
			if err := mgr.ImportImageFromReader(ctxT(t), bytes.NewReader(rawImage()), name); err == nil {
				t.Error("a second import under the same name succeeded")
			}
			if got, err := mgr.VolumeSHA256(ctxT(t), storage.DefaultImagesPool, name); err != nil || got != digestOf(content) {
				t.Errorf("after a refused second import the image reads %s, %v", got, err)
			}
		})
	}
	t.Run("backing file refused", func(t *testing.T) {
		name := uniqueName(t) + ".qcow2"
		if err := importImage(t, mgr, name, qcow2Header(2<<20, 300)); err == nil || !strings.Contains(err.Error(), "backing file") {
			t.Errorf("err = %v, want a backing file refusal", err)
		}
		if imageExists(t, mgr, name) {
			t.Error("a refused image left its volume")
		}
	})
	t.Run("failing reader", func(t *testing.T) {
		name := uniqueName(t) + ".raw"
		r := io.MultiReader(bytes.NewReader(rawImage()), iotest.ErrReader(errors.New("source gone")))
		if err := mgr.ImportImageFromReader(ctxT(t), r, name); err == nil {
			t.Error("an import from a failing reader succeeded")
		}
		if imageExists(t, mgr, name) {
			t.Error("a failed import left its volume")
		}
	})
}

// integrationSpec is a small VM on an image, attached to bridge.
func integrationSpec(t *testing.T, name, bridge, image, digest, extra string) *v1alpha1.VirtualMachine {
	t.Helper()
	spec := fmt.Sprintf(`apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: %s
spec:
  vcpus: 1
  memoryGiB: 1
  autostart: false
  bootDisk: {sizeGB: 1, image: %s, imageSHA256: %s}
  networkInterfaces:
    - {ip: 192.0.2.10/24, gateway: 192.0.2.1, bridge: %s, hostDevice: %s}
%s`, name, image, digest, bridge, shortName("fdryvm", name), extra)
	v, err := loader.LoadFromYAML([]byte(spec))
	if err != nil {
		t.Fatalf("load spec: %v\n%s", err, spec)
	}
	return v
}

// TestIntegrationLifecycle runs one VM through create, get, list and
// destroy. Its user data and an input disk's file hold characters that are
// special in XML, and the file starts with a tab, all of which the stored
// spec must keep; destroy must leave none of its volumes.
func TestIntegrationLifecycle(t *testing.T) {
	lv, mgr := setup(t)
	requireKVM(t, lv)
	name := uniqueName(t)
	bridge := isolatedNetwork(t, lv, name)
	image := name + "-image.raw"
	content := rawImage()
	if err := importImage(t, mgr, image, content); err != nil {
		t.Fatalf("import image %s: %v", image, err)
	}

	const userData = "#!/bin/sh\ntrue && echo '<ok>' > /dev/null\n"
	const inputFile = "\t<node a='1'/>\n& more\n"
	v := integrationSpec(t, name, bridge, image, digestOf(content), fmt.Sprintf(`  dataDisks:
    - {device: vdb, sizeGB: 1, serial: data-0}
  cloudInit:
    rawUserData: %q
  inputDisks:
    - label: SEED
      files:
        - {name: node.xml, string: %q}
`, userData, inputFile))

	destroyed := false
	destroyAtEnd(t, lv, name, &destroyed)
	if err := vm.CreateFromConfig(ctxT(t), v); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("read", func(t *testing.T) {
		got, err := vm.GetVM(ctxT(t), name)
		if err != nil {
			t.Fatalf("GetVM: %v", err)
		}
		if got.Status.Phase != v1alpha1.VMPhaseRunning {
			t.Errorf("phase = %s, want %s", got.Status.Phase, v1alpha1.VMPhaseRunning)
		}
		if got.Spec.CloudInit == nil || got.Spec.CloudInit.RawUserData != userData {
			t.Errorf("stored user data does not read back unchanged: %+v", got.Spec.CloudInit)
		}
		if len(got.Spec.InputDisks) != 1 || len(got.Spec.InputDisks[0].Files) != 1 ||
			got.Spec.InputDisks[0].Files[0].String != inputFile {
			t.Errorf("stored input disk does not read back unchanged: %+v", got.Spec.InputDisks)
		}
		list, err := vm.ListVMs(ctxT(t))
		if err != nil {
			t.Fatalf("ListVMs: %v", err)
		}
		found := false
		for _, l := range list {
			found = found || l.Name == name
		}
		if !found {
			t.Errorf("ListVMs does not have %s", name)
		}
		if vols := vmVolumes(t, mgr, name); len(vols) != 4 {
			t.Errorf("volumes %v, want boot, data, cloud-init and input", vols)
		}
	})

	t.Run("destroy", func(t *testing.T) {
		if err := vm.Destroy(ctxT(t), name, false); err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		destroyed = true
		if _, err := vm.GetVM(ctxT(t), name); !libvirt.IsNotFound(err) {
			t.Errorf("GetVM after destroy = %v, want not found", err)
		}
		if vols := vmVolumes(t, mgr, name); len(vols) != 0 {
			t.Errorf("volumes left after destroy: %v", vols)
		}
		if !imageExists(t, mgr, image) {
			t.Errorf("image %s was deleted with the VM", image)
		}
	})
}

// TestIntegrationCreateDigest: a volume that is not the image the spec's
// digest names (other content, or cut short as a crash mid-import leaves
// it) fails the create before it makes anything.
func TestIntegrationCreateDigest(t *testing.T) {
	lv, mgr := setup(t)
	bridge := isolatedNetwork(t, lv, uniqueName(t))
	full := rawImage()
	for name, stored := range map[string][]byte{"other": append([]byte{1}, full[1:]...), "truncated": full[:len(full)/2]} {
		t.Run(name, func(t *testing.T) {
			vmName := uniqueName(t)
			image := vmName + "-image.raw"
			if err := importImage(t, mgr, image, stored); err != nil {
				t.Fatalf("import: %v", err)
			}
			destroyed := false
			destroyAtEnd(t, lv, vmName, &destroyed)
			err := vm.CreateFromConfig(ctxT(t), integrationSpec(t, vmName, bridge, image, digestOf(full), ""))
			if !errors.Is(err, storage.ErrImageDigest) {
				t.Errorf("err = %v, want ErrImageDigest", err)
			}
			if _, err := lv.DomainLookupByName(vmName); !libvirt.IsNotFound(err) {
				t.Errorf("the domain exists, or its lookup failed: %v", err)
			}
			if vols := vmVolumes(t, mgr, vmName); len(vols) != 0 {
				t.Errorf("a refused create left volumes: %v", vols)
			}
		})
	}
}

// TestIntegrationSerialLog: a VM's serial console is logged to its file in
// the logs pool from its first boot (the firmware prints to the serial
// console), logs reads it through libvirt, the file is private to its owner,
// and destroy deletes it.
func TestIntegrationSerialLog(t *testing.T) {
	lv, mgr := setup(t)
	requireKVM(t, lv)
	name := uniqueName(t)
	bridge := isolatedNetwork(t, lv, name)
	image := name + "-image.raw"
	content := rawImage()
	if err := importImage(t, mgr, image, content); err != nil {
		t.Fatalf("import image %s: %v", image, err)
	}
	destroyed := false
	destroyAtEnd(t, lv, name, &destroyed)
	if err := vm.CreateFromConfig(ctxT(t), integrationSpec(t, name, bridge, image, digestOf(content), "")); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The firmware finds nothing to boot on the image and says so on the
	// serial console; wait for the first bytes
	var out bytes.Buffer
	deadline := time.Now().Add(time.Minute)
	for {
		out.Reset()
		err := vm.Logs(ctxT(t), name, vm.LogSerial, &out)
		if err == nil && out.Len() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no serial output within a minute: %v (%d bytes)", err, out.Len())
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("serial log starts: %q", firstBytes(out.Bytes(), 200))

	t.Run("file", func(t *testing.T) {
		dir, err := mgr.LogsDir(ctxT(t))
		if err != nil {
			t.Fatalf("LogsDir: %v", err)
		}
		vols, err := mgr.ListVolumes(ctxT(t), storage.DefaultLogsPool)
		if err != nil {
			t.Fatalf("list logs pool: %v", err)
		}
		want := dir + "/" + name + "-serial.log"
		for _, v := range vols {
			if v.Name != name+"-serial.log" {
				continue
			}
			if v.Path != want {
				t.Errorf("log at %s, want %s", v.Path, want)
			}
			pool, _ := lv.StoragePoolLookupByName(storage.DefaultLogsPool)
			vol, err := lv.StorageVolLookupByName(pool, v.Name)
			if err != nil {
				t.Fatalf("look up log volume: %v", err)
			}
			xml, err := lv.StorageVolGetXMLDesc(vol, 0)
			if err != nil {
				t.Fatalf("log volume XML: %v", err)
			}
			var def libvirtxml.StorageVolume
			if err := def.Unmarshal(xml); err != nil {
				t.Fatalf("parse log volume XML: %v", err)
			}
			if def.Target == nil || def.Target.Permissions == nil {
				t.Fatalf("log volume has no permissions: %s", xml)
			}
			p := def.Target.Permissions
			t.Logf("log file mode %s, owner %s, group %s, label %q", p.Mode, p.Owner, p.Group, p.Label)
			if p.Mode != "0600" {
				t.Errorf("log file mode %s, want 0600", p.Mode)
			}
			return
		}
		t.Fatalf("no volume %s-serial.log in %s", name, storage.DefaultLogsPool)
	})

	t.Run("destroy deletes it", func(t *testing.T) {
		if err := vm.Destroy(ctxT(t), name, false); err != nil {
			t.Fatalf("Destroy: %v", err)
		}
		destroyed = true
		if err := mgr.RefreshPool(ctxT(t), storage.DefaultLogsPool); err != nil {
			t.Fatalf("refresh logs pool: %v", err)
		}
		vols, err := mgr.ListVolumes(ctxT(t), storage.DefaultLogsPool)
		if err != nil {
			t.Fatalf("list logs pool: %v", err)
		}
		for _, v := range vols {
			if strings.HasPrefix(v.Name, name+"-serial.log") {
				t.Errorf("left after destroy: %s", v.Name)
			}
		}
		if err := vm.Logs(ctxT(t), name, vm.LogSerial, io.Discard); err == nil {
			t.Error("logs after destroy succeeded")
		}
	})
}

// firstBytes is at most n bytes of b.
func firstBytes(b []byte, n int) []byte {
	return b[:min(n, len(b))]
}
