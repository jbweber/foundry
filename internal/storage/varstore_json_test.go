package storage

import (
	"context"
	"strings"
	"testing"
)

// jsonVarstoreOK is the shape of the stores virt-fw-vars --output-json writes.
const jsonVarstoreOK = `{"version": 2, "variables": [
  {"name": "PK", "guid": "8be4df61-93ca-11d2-aa0d-00e098032b8c", "attr": 39, "data": "aa55", "time": "e50a00"},
  {"name": "SecureBootEnable", "guid": "f0a30bc7-af08-4556-99c4-001009c93a44", "attr": 3, "data": "01"},
  {"name": "CustomMode", "guid": "c076ec0c-7028-4399-a072-71ee5c448b9f", "attr": 3, "data": "00"}]}`

func TestValidateVarstoreJSON(t *testing.T) {
	const noPK = `{"version": 2, "variables": [{"name": "SecureBootEnable", "guid": "f0a30bc7-af08-4556-99c4-001009c93a44", "attr": 3, "data": "01"}]}`
	const emptyPK = `{"version": 2, "variables": [{"name": "PK", "guid": "8be4df61-93ca-11d2-aa0d-00e098032b8c", "attr": 39, "data": ""},
  {"name": "SecureBootEnable", "guid": "f0a30bc7-af08-4556-99c4-001009c93a44", "attr": 3, "data": "01"}]}`
	const secureBootOff = `{"version": 2, "variables": [{"name": "PK", "guid": "8be4df61-93ca-11d2-aa0d-00e098032b8c", "attr": 39, "data": "aa"},
  {"name": "SecureBootEnable", "guid": "f0a30bc7-af08-4556-99c4-001009c93a44", "attr": 3, "data": "00"}]}`
	tests := []struct {
		name, data, wantErr string
	}{
		{"enrolled", jsonVarstoreOK, ""},
		{"blank template", `{"version": 2, "variables": []}`, "would not enforce"},
		{"no PK", noPK, "would not enforce"},
		{"empty PK", emptyPK, "would not enforce"},
		{"Secure Boot off", secureBootOff, "would not enforce"},
		{"PK of another namespace", strings.Replace(jsonVarstoreOK, "8be4df61", "00000000", 1), "would not enforce"},
		{"other version", strings.Replace(jsonVarstoreOK, `"version": 2`, `"version": 1`, 1), "version must be 2"},
		{"no version", `{"variables": []}`, "version must be 2"},
		{"not JSON", `{nope`, "not a JSON varstore"},
		{"more after the object", jsonVarstoreOK + `{}`, "more follows"},
		{"a closing brace after the object", jsonVarstoreOK + `}`, "more follows"},
		{"a field QEMU does not know", `{"version": 2, "variables": [], "extra": 1}`, "unknown field"},
		{"bad GUID", strings.Replace(jsonVarstoreOK, "8be4df61-93ca", "nope", 1), "not a GUID"},
		{"data not hex", strings.Replace(jsonVarstoreOK, "aa55", "zz", 1), "data is not hex"},
		{"time not hex", strings.Replace(jsonVarstoreOK, "e50a00", "z", 1), "time is not hex"},
		{"no attr", `{"version": 2, "variables": [{"name": "PK", "guid": "8be4df61-93ca-11d2-aa0d-00e098032b8c", "data": "aa"}]}`, "name and attr"},
		{"too big", jsonVarstoreOK + strings.Repeat(" ", MaxVarstoreSize), "more than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVarstoreJSON([]byte(tt.data))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateVarstoreJSON() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateVarstoreJSON() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// Only aarch64 takes the JSON form; x86_64 answers for it as for anything else
// that is not a raw varstore.
func TestValidateVarstoreFor(t *testing.T) {
	raw := testVarstore(4096)
	for _, arch := range []string{"x86_64", "aarch64"} {
		if err := ValidateVarstoreFor(arch, raw); err != nil {
			t.Errorf("raw store for %s: %v", arch, err)
		}
	}
	if err := ValidateVarstoreFor("aarch64", []byte(jsonVarstoreOK)); err != nil {
		t.Errorf("JSON store for aarch64: %v", err)
	}
	want := ValidateVarstore([]byte(jsonVarstoreOK))
	if err := ValidateVarstoreFor("x86_64", []byte(jsonVarstoreOK)); err == nil || want == nil || err.Error() != want.Error() {
		t.Errorf("JSON store for x86_64 = %v, want the raw check's %v", err, want)
	}
}

func TestReadVarstore_JSON(t *testing.T) {
	got, err := ReadVarstore(strings.NewReader(jsonVarstoreOK))
	if err != nil || string(got) != jsonVarstoreOK {
		t.Fatalf("ReadVarstore() = %q, %v", got, err)
	}
	if _, err := ReadVarstore(strings.NewReader(`{"version": 2, "variables": []}`)); err == nil {
		t.Fatal("ReadVarstore() took a store with no keys")
	}
}

func TestManager_ImportVarstore_JSON(t *testing.T) {
	ctx := context.Background()
	mock := newMockLibvirtClient()
	mgr := NewManager(mock)
	if err := mgr.CreatePool(ctx, DefaultImagesPool, PoolTypeDir, DefaultImagesPath); err != nil {
		t.Fatal(err)
	}
	vols := func() map[string]*mockVolume { return mock.volumes[DefaultImagesPool] }

	if err := mgr.ImportVarstore(ctx, "corp-keys", "x86_64", []byte(jsonVarstoreOK), false); err == nil {
		t.Fatal("imported a JSON store for x86_64")
	}
	if err := mgr.ImportVarstore(ctx, "corp-keys", "aarch64", []byte(jsonVarstoreOK), false); err != nil {
		t.Fatalf("ImportVarstore() = %v", err)
	}
	vol := vols()["varstore_corp-keys_aarch64.json"]
	if vol == nil || string(vol.data) != jsonVarstoreOK {
		t.Fatalf("JSON volume missing or with other content: %+v", vol)
	}

	// A profile has one store per architecture, whatever its form
	raw := testVarstore(4096)
	if err := mgr.ImportVarstore(ctx, "corp-keys", "aarch64", raw, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("flash store over a JSON one = %v, want an \"already exists\" error", err)
	}
	if err := mgr.ImportVarstore(ctx, "corp-keys", "aarch64", raw, true); err != nil {
		t.Fatalf("replace with a flash store: %v", err)
	}
	if _, ok := vols()["varstore_corp-keys_aarch64.json"]; ok {
		t.Fatal("replace left the JSON store")
	}
	if err := mgr.ImportVarstore(ctx, "corp-keys", "aarch64", []byte(jsonVarstoreOK), false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("JSON store over a flash one = %v, want an \"already exists\" error", err)
	}
	if err := mgr.ImportVarstore(ctx, "corp-keys", "aarch64", []byte(jsonVarstoreOK), true); err != nil {
		t.Fatalf("replace with a JSON store: %v", err)
	}
	if _, ok := vols()["varstore_corp-keys_aarch64.fd"]; ok {
		t.Fatal("replace left the flash store")
	}

	got, err := mgr.ListVarstores(ctx)
	if err != nil || len(got) != 1 || got[0].Profile != "corp-keys" || got[0].Arch != "aarch64" || got[0].Volume != "varstore_corp-keys_aarch64.json" {
		t.Fatalf("ListVarstores() = %+v, %v", got, err)
	}

	if err := mgr.DeleteVarstore(ctx, "corp-keys", "aarch64"); err != nil {
		t.Fatal(err)
	}
	if len(vols()) != 0 {
		t.Fatalf("DeleteVarstore() left %v", vols())
	}
	if err := mgr.DeleteVarstore(ctx, "corp-keys", "aarch64"); err == nil {
		t.Fatal("deleting a missing varstore succeeded")
	}
}
