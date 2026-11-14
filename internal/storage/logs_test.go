package storage

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/digitalocean/go-libvirt"
)

// logsPool defines the logs pool in the mock with the given XML and state.
func logsPool(m *mockLibvirtClient, xml string, state libvirt.StoragePoolState) {
	m.pools[DefaultLogsPool] = &mockPool{name: DefaultLogsPool, uuid: "mock-uuid-logs", state: state, xmlDesc: xml}
	m.volumes[DefaultLogsPool] = map[string]*mockVolume{}
}

// TestLogsDir: the logs pool's directory is its target path as libvirt
// reports it, wherever that is; a pool the domain XML cannot name a file in,
// or foundry could not read back, is refused, saying why.
func TestLogsDir(t *testing.T) {
	dirPool := func(path string) string {
		return "<pool type='dir'><name>foundry-logs</name><target><path>" + path + "</path></target></pool>"
	}
	tests := []struct {
		name    string
		xml     string
		state   libvirt.StoragePoolState
		want    string
		wantErr string
	}{
		{name: "default path", xml: dirPool(DefaultLogsPath), state: libvirt.StoragePoolRunning, want: DefaultLogsPath},
		{name: "defined elsewhere", xml: dirPool("/srv/logs"), state: libvirt.StoragePoolRunning, want: "/srv/logs"},
		{name: "not running", xml: dirPool(DefaultLogsPath), state: libvirt.StoragePoolInactive, wantErr: "not running"},
		{name: "not a dir pool", xml: "<pool type='logical'><name>foundry-logs</name><target><path>/dev/vg</path></target></pool>",
			state: libvirt.StoragePoolRunning, wantErr: "dir pool"},
		{name: "relative path", xml: dirPool("logs"), state: libvirt.StoragePoolRunning, wantErr: "absolute"},
		{name: "unclean path", xml: dirPool("/srv/../logs"), state: libvirt.StoragePoolRunning, wantErr: "clean"},
		{name: "root", xml: dirPool("/"), state: libvirt.StoragePoolRunning, wantErr: "absolute"},
		{name: "no target", xml: "<pool type='dir'><name>foundry-logs</name></pool>", state: libvirt.StoragePoolRunning, wantErr: "absolute"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockLibvirtClient()
			logsPool(m, tt.xml, tt.state)
			got, err := NewManager(m).LogsDir(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LogsDir() = %q, %v; want an error with %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("LogsDir() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}

	t.Run("no pool", func(t *testing.T) {
		if _, err := NewManager(newMockLibvirtClient()).LogsDir(context.Background()); err == nil {
			t.Fatal("LogsDir() without a logs pool succeeded")
		}
	})
}

// TestDownloadVolume: the whole volume is written; a missing volume is an
// error; an ended context stops the download with its error.
func TestDownloadVolume(t *testing.T) {
	m := newMockLibvirtClient()
	logsPool(m, "", libvirt.StoragePoolRunning)
	m.volumes[DefaultLogsPool]["web-serial.log"] = &mockVolume{name: "web-serial.log", data: []byte("boot\nlogin: ")}
	mgr := NewManager(m)

	var buf bytes.Buffer
	if err := mgr.DownloadVolume(context.Background(), DefaultLogsPool, "web-serial.log", &buf); err != nil {
		t.Fatalf("DownloadVolume() error = %v", err)
	}
	if buf.String() != "boot\nlogin: " {
		t.Errorf("DownloadVolume() wrote %q", buf.String())
	}

	if err := mgr.DownloadVolume(context.Background(), DefaultLogsPool, "db-serial.log", &buf); err == nil {
		t.Error("DownloadVolume() of a missing volume succeeded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.DownloadVolume(ctx, DefaultLogsPool, "web-serial.log", &buf); !errors.Is(err, context.Canceled) {
		t.Errorf("DownloadVolume() with an ended context = %v, want context.Canceled", err)
	}
}

// TestEnsurePool_ExistingChecked: an existing pool is kept where it is, at
// any absolute, clean path, stopped or running; one foundry could not use
// (not a dir pool, a relative or unclean path) is an error, and nothing is
// redefined.
func TestEnsurePool_ExistingChecked(t *testing.T) {
	for _, tt := range []struct {
		name, xml string
		state     libvirt.StoragePoolState
		wantErr   string
	}{
		{"other path", "<pool type='dir'><name>foundry-vms</name><target><path>/srv/vms</path></target></pool>", libvirt.StoragePoolRunning, ""},
		{"stopped", "<pool type='dir'><name>foundry-vms</name><target><path>/srv/vms</path></target></pool>", libvirt.StoragePoolInactive, ""},
		{"not dir", "<pool type='netfs'><name>foundry-vms</name><target><path>/mnt/vms</path></target></pool>", libvirt.StoragePoolRunning, "dir pools only"},
		{"relative", "<pool type='dir'><name>foundry-vms</name><target><path>vms</path></target></pool>", libvirt.StoragePoolRunning, "absolute"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockLibvirtClient()
			m.pools[DefaultVMsPool] = &mockPool{name: DefaultVMsPool, state: tt.state, xmlDesc: tt.xml}
			m.volumes[DefaultVMsPool] = map[string]*mockVolume{}
			err := NewManager(m).EnsurePool(context.Background(), DefaultVMsPool, PoolTypeDir, DefaultVMsPath)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("EnsurePool() = %v, want the existing pool kept", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("EnsurePool() = %v, want an error with %q", err, tt.wantErr)
			}
			if m.pools[DefaultVMsPool].xmlDesc != tt.xml {
				t.Error("the existing pool was redefined")
			}
		})
	}
}

// TestCheckDirPool: the check LogsDir makes applies to any pool by name.
func TestCheckDirPool(t *testing.T) {
	m := newMockLibvirtClient()
	m.pools["fast"] = &mockPool{name: "fast", state: libvirt.StoragePoolRunning,
		xmlDesc: "<pool type='dir'><name>fast</name><target><path>/ssd/vms</path></target></pool>"}
	mgr := NewManager(m)
	if dir, err := mgr.CheckDirPool(context.Background(), "fast"); err != nil || dir != "/ssd/vms" {
		t.Errorf("CheckDirPool(fast) = %q, %v", dir, err)
	}
	if _, err := mgr.CheckDirPool(context.Background(), "missing"); err == nil {
		t.Error("CheckDirPool of a missing pool succeeded")
	}
}
