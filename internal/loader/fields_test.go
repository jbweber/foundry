package loader

import (
	"strings"
	"testing"
)

// fieldsBase is a valid spec whose first interface's ip and gateway are IP
// and GW, followed by extra (spec-level YAML).
const fieldsBase = `
apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: a
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 50
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: IP
      gateway: GW
      bridge: br0
`

// TestLoadFromYAML_AddressFields: ip is an IPv4 address with a prefix length
// (the MAC and host-side device name are derived from it), gateway an IP
// address. A bad value fails at load, not after volumes were made.
func TestLoadFromYAML_AddressFields(t *testing.T) {
	for _, tt := range []struct{ name, ip, gw, wantErr string }{
		{"valid", "10.0.0.1/24", "10.0.0.254", ""},
		{"ip is not an address", "not-an-ip", "10.0.0.254", ".ip \"not-an-ip\" must be an IPv4 address"},
		{"ip without a prefix length", "10.0.0.1", "10.0.0.254", "must be an IPv4 address with a prefix length"},
		{"ip is IPv6", "fd00::5/64", "fd00::1", "must be an IPv4 address"},
		{"gateway is not an address", "10.0.0.1/24", "gw", ".gateway \"gw\" is not an IP address"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := strings.NewReplacer("IP", tt.ip, "GW", tt.gw).Replace(fieldsBase)
			_, err := LoadFromYAML([]byte(spec))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestLoadFromYAML_DuplicateAddressOtherPrefix: 10.0.0.5/24 and 10.0.0.5/16
// are the same address, which would give two interfaces the same MAC.
func TestLoadFromYAML_DuplicateAddressOtherPrefix(t *testing.T) {
	spec := strings.NewReplacer("IP", "10.0.0.5/24", "GW", "10.0.0.1").Replace(fieldsBase) +
		"    - ip: 10.0.0.5/16\n      gateway: 10.0.0.1\n      bridge: br1\n"
	_, err := LoadFromYAML([]byte(spec))
	if err == nil || !strings.Contains(err.Error(), "is duplicated") {
		t.Fatalf("got %v, want a duplicated-address error", err)
	}
}

// TestLoadFromYAML_PasswordHashOneLine: cloud-init passes the hash to
// chpasswd as one line; a line break would add lines of its own.
func TestLoadFromYAML_PasswordHashOneLine(t *testing.T) {
	base := strings.NewReplacer("IP", "10.0.0.1/24", "GW", "10.0.0.254").Replace(fieldsBase)
	if _, err := LoadFromYAML([]byte(base + "  cloudInit:\n    passwordHash: \"$6$salt$hash\"\n")); err != nil {
		t.Fatalf("one-line hash refused: %v", err)
	}
	for _, h := range []string{`$6$salt$hash\nroot:x`, `$6$salt$hash\r`} {
		_, err := LoadFromYAML([]byte(base + "  cloudInit:\n    passwordHash: \"" + h + "\"\n"))
		if err == nil || !strings.Contains(err.Error(), "passwordHash must be a single line") {
			t.Errorf("hash %q: got %v, want a single-line error", h, err)
		}
	}
}

// TestLoadFromYAML_FwCfgNameCharacters: a fw_cfg name is "opt/" and
// characters QEMU's option syntax takes unescaped; a comma would split the
// option and the VM would not start.
func TestLoadFromYAML_FwCfgNameCharacters(t *testing.T) {
	base := strings.NewReplacer("IP", "10.0.0.1/24", "GW", "10.0.0.254").Replace(fieldsBase)
	for name, ok := range map[string]bool{
		"opt/com.example/config-1_a": true,
		"opt/x,y":                    false,
		"opt/x y":                    false,
		"opt/x=y":                    false,
		"opt/":                       false,
	} {
		_, err := LoadFromYAML([]byte(base + "  fwCfg:\n    - name: \"" + name + "\"\n      string: v\n"))
		if ok && err != nil {
			t.Errorf("name %q refused: %v", name, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), "spec.fwCfg[0].name")) {
			t.Errorf("name %q: got %v, want a spec.fwCfg[0].name error", name, err)
		}
	}
}
