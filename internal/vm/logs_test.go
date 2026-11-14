package vm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
	"github.com/jbweber/foundry/internal/storage"
)

// logContent makes the mock's DownloadVolume write "<volume>;" for each
// volume, so the order of the reads shows in the output.
func logContent(sm *mockStorageManager) {
	sm.downloadVolumeFunc = func(_ context.Context, _, volume string, w io.Writer) error {
		_, err := io.WriteString(w, volume+";")
		return err
	}
}

// TestLogsWithDeps: the rotated copies are written oldest first (highest
// number first, numerically, so .10 before .9), then the log itself; only
// the VM's own files are read, after a refresh of the logs pool.
func TestLogsWithDeps(t *testing.T) {
	_, sm := poolMocks(
		"foundry-logs/web-serial.log",
		"foundry-logs/web-serial.log.0",
		"foundry-logs/web-serial.log.10",
		"foundry-logs/web-serial.log.9",
		"foundry-logs/web-serial.log.bak",
		"foundry-logs/web-2-serial.log",
		"foundry-vms/web-serial.log",
	)
	logContent(sm)
	var out bytes.Buffer
	if err := logsWithDeps(context.Background(), "web", sm, &out); err != nil {
		t.Fatalf("logsWithDeps: %v", err)
	}
	want := "web-serial.log.10;web-serial.log.9;web-serial.log.0;web-serial.log;"
	if out.String() != want {
		t.Errorf("wrote %q, want %q", out.String(), want)
	}
	if len(sm.refreshPoolCalls) != 1 || sm.refreshPoolCalls[0] != storage.DefaultLogsPool {
		t.Errorf("refreshed %v, want the logs pool once", sm.refreshPoolCalls)
	}
}

// TestLogsWithDeps_Errors: no log, no logs pool and a failed read are
// errors that say so.
func TestLogsWithDeps_Errors(t *testing.T) {
	t.Run("no log", func(t *testing.T) {
		_, sm := poolMocks("foundry-logs/other-serial.log")
		err := logsWithDeps(context.Background(), "web", sm, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "no serial log for web") {
			t.Errorf("err = %v, want no serial log", err)
		}
	})
	t.Run("no logs pool", func(t *testing.T) {
		sm := newMockStorageManager()
		sm.refreshPoolFunc = func(context.Context, string) error {
			return libvirt.Error{Code: uint32(libvirt.ErrNoStoragePool), Message: "Storage pool not found"}
		}
		err := logsWithDeps(context.Background(), "web", sm, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "there is no pool foundry-logs") {
			t.Errorf("err = %v, want no pool", err)
		}
	})
	t.Run("read fails", func(t *testing.T) {
		_, sm := poolMocks("foundry-logs/web-serial.log")
		sm.downloadVolumeFunc = func(context.Context, string, string, io.Writer) error { return errors.New("gone") }
		if err := logsWithDeps(context.Background(), "web", sm, io.Discard); err == nil {
			t.Error("a failed read succeeded")
		}
	})
}

// TestLogs_UnknownKind: serial is the only log; another is refused before
// any connection.
func TestLogs_UnknownKind(t *testing.T) {
	err := Logs(context.Background(), "web", "install", io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown log "install"`) {
		t.Errorf("err = %v, want unknown log", err)
	}
}

// TestDestroyWithDeps_DeletesSerialLog: destroy deletes the VM's serial log
// and its rotated copies, by exact name, and no other VM's log.
func TestDestroyWithDeps_DeletesSerialLog(t *testing.T) {
	lv, sm := poolMocks(
		"foundry-vms/web_boot.qcow2",
		"foundry-logs/web-serial.log",
		"foundry-logs/web-serial.log.0",
		"foundry-logs/web-serial.log.bak",
		"foundry-logs/web-2-serial.log",
		"foundry-logs/we-serial.log",
	)
	withSpec(t, lv, testVMConfigNamed("web"))
	if err := destroyWithDeps(context.Background(), "web", lv, sm, false); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	want := "foundry-logs/web-serial.log foundry-logs/web-serial.log.0 foundry-vms/web_boot.qcow2"
	if got := deleted(sm); got != want {
		t.Errorf("deleted %s, want %s", got, want)
	}
}

// TestDestroyWithDeps_LeftoverSerialLog: with no domain, a serial log left
// of the VM is removed as other leftovers are.
func TestDestroyWithDeps_LeftoverSerialLog(t *testing.T) {
	lv, sm := poolMocks("foundry-logs/web-serial.log", "foundry-logs/web-2-serial.log")
	noDomain(lv)
	if err := destroyWithDeps(context.Background(), "web", lv, sm, false); err != nil {
		t.Fatalf("destroy of leftovers: %v", err)
	}
	if got := deleted(sm); got != "foundry-logs/web-serial.log" {
		t.Errorf("deleted %s, want web's serial log only", got)
	}
}

// TestDestroyWithDeps_LogsPoolUnreadable: a logs pool that cannot be read
// does not stop the destroy; the VM's volumes still go.
func TestDestroyWithDeps_LogsPoolUnreadable(t *testing.T) {
	lv, sm := poolMocks("foundry-vms/web_boot.qcow2")
	sm.refreshPoolFunc = func(context.Context, string) error { return errors.New("pool not found") }
	withSpec(t, lv, testVMConfigNamed("web"))
	if err := destroyWithDeps(context.Background(), "web", lv, sm, false); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := deleted(sm); got != "foundry-vms/web_boot.qcow2" {
		t.Errorf("deleted %s", got)
	}
}

// TestCreateFromConfigWithDeps_SerialLog: the domain XML names the serial log
// in the logs pool's directory as libvirt reports it; a log left of an
// earlier VM of the name, or a logs pool LogsDir refuses, fails the create
// before anything is made.
func TestCreateFromConfigWithDeps_SerialLog(t *testing.T) {
	t.Run("pool directory", func(t *testing.T) {
		lv := newMockLibvirtClient()
		sm := newMockStorageManager()
		sm.logsDirFunc = func(context.Context) (string, error) { return "/srv/logs", nil }
		if err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv)); err != nil {
			t.Fatalf("create: %v", err)
		}
		if len(lv.domainDefineXMLCalls) != 1 || !strings.Contains(lv.domainDefineXMLCalls[0], `<log file="/srv/logs/test-vm-serial.log" append="on">`) {
			t.Errorf("domain XML has no serial log in /srv/logs: %v", lv.domainDefineXMLCalls)
		}
	})
	for name, setup := range map[string]func(*mockStorageManager){
		"old log": func(sm *mockStorageManager) {
			sm.listVolumesFunc = func(_ context.Context, pool string) ([]storage.VolumeInfo, error) {
				if pool == storage.DefaultLogsPool {
					return []storage.VolumeInfo{{Name: "test-vm-serial.log.0"}}, nil
				}
				return nil, nil
			}
		},
		"logs pool refused": func(sm *mockStorageManager) {
			sm.logsDirFunc = func(context.Context) (string, error) { return "", errors.New("logs pool foundry-logs is not running") }
		},
	} {
		t.Run(name, func(t *testing.T) {
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			setup(sm)
			if err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv)); err == nil {
				t.Fatal("create succeeded")
			}
			if len(sm.createVolumeCalls)+len(lv.domainDefineXMLCalls) != 0 {
				t.Errorf("a refused create made volumes or a domain")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_CleanupDeletesSerialLog: a create that fails
// after it defined (and may have started) the domain deletes the serial log
// virtlogd made; one that fails before the define leaves the logs pool alone.
func TestCreateFromConfigWithDeps_CleanupDeletesSerialLog(t *testing.T) {
	for name, tt := range map[string]struct {
		fail func(*mockLibvirtClient)
		want bool
	}{
		"start fails": {func(lv *mockLibvirtClient) {
			lv.domainCreateFunc = func(libvirt.Domain) error { return errors.New("no memory") }
		}, true},
		"define fails": {func(lv *mockLibvirtClient) {
			lv.domainDefineXMLFunc = func(string) (libvirt.Domain, error) { return libvirt.Domain{}, errors.New("bad") }
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			lv := newMockLibvirtClient()
			tt.fail(lv)
			sm := newMockStorageManager()
			started := false
			sm.listVolumesFunc = func(_ context.Context, pool string) ([]storage.VolumeInfo, error) {
				// The log appears once the domain was defined (and started)
				if pool == storage.DefaultLogsPool && len(lv.domainDefineXMLCalls) > 0 {
					started = true
					return []storage.VolumeInfo{{Name: "test-vm-serial.log"}, {Name: "other-serial.log"}}, nil
				}
				return nil, nil
			}
			if err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv)); err == nil {
				t.Fatal("create succeeded")
			}
			got := hasCall(sm.deleteVolumeCalls, "foundry-logs/test-vm-serial.log")
			if got != tt.want {
				t.Errorf("serial log deleted = %v, want %v (deleted %v, log seen %v)", got, tt.want, sm.deleteVolumeCalls, started)
			}
			if hasCall(sm.deleteVolumeCalls, "foundry-logs/other-serial.log") {
				t.Error("deleted another VM's serial log")
			}
		})
	}
}

// testVMConfigNamed is testVMConfig with another name.
func testVMConfigNamed(name string) *v1alpha1.VirtualMachine {
	vm := testVMConfig()
	vm.Name = name
	return vm
}
