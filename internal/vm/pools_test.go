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
