package vm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jbweber/foundry/internal/storage"
)

// TestCreateFromConfigWithDeps_ChecksPools: a default pool create cannot use
// (stopped, not a dir pool, a bad path: whatever CheckDirPool refuses) fails
// the create before anything is made.
func TestCreateFromConfigWithDeps_ChecksPools(t *testing.T) {
	for _, bad := range []string{storage.DefaultImagesPool, storage.DefaultVMsPool} {
		t.Run(bad, func(t *testing.T) {
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			sm.checkDirPoolFunc = func(_ context.Context, name string) (string, error) {
				if name == bad {
					return "", errors.New("pool " + name + " is not running")
				}
				return "/pools/" + name, nil
			}
			err := createFromConfigWithDeps(context.Background(), testVMConfig(), lv, sm, newMockMetadataClient(lv))
			if err == nil || !strings.Contains(err.Error(), bad) {
				t.Fatalf("err = %v, want one naming %s", err, bad)
			}
			if len(sm.createVolumeCalls)+len(lv.domainDefineXMLCalls) != 0 {
				t.Error("a refused create made volumes or a domain")
			}
		})
	}
}

// TestCreateFromConfigWithDeps_ChecksStoragePool: spec.storagePool may name
// any running dir pool; one that does not exist, is stopped or is not a dir
// pool fails the create, naming the field, before anything is made.
func TestCreateFromConfigWithDeps_ChecksStoragePool(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"usable", nil, false},
		{"missing", errors.New("pool fast: storage pool not found"), true},
		{"stopped", errors.New("pool fast is not running"), true},
		{"not a dir pool", errors.New(`pool fast is a "logical" pool; foundry uses dir pools only`), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vm := testVMConfig()
			vm.Spec.StoragePool = "fast"
			lv := newMockLibvirtClient()
			sm := newMockStorageManager()
			sm.checkDirPoolFunc = func(_ context.Context, name string) (string, error) {
				if name == "fast" && tt.err != nil {
					return "", tt.err
				}
				return "/pools/" + name, nil
			}
			err := createFromConfigWithDeps(context.Background(), vm, lv, sm, newMockMetadataClient(lv))
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("create in a usable pool: %v", err)
				}
				if !hasCall(sm.checkDirPoolCalls, "fast") {
					t.Error("spec.storagePool was not checked")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "spec.storagePool") {
				t.Fatalf("err = %v, want one naming spec.storagePool", err)
			}
			if len(sm.createVolumeCalls)+len(lv.domainDefineXMLCalls) != 0 {
				t.Error("a refused create made volumes or a domain")
			}
		})
	}
}
