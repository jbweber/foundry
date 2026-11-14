package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const inputDiskBase = `apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata:
  name: input-test
spec:
  vcpus: 1
  memoryGiB: 1
  bootDisk: {sizeGB: 10, empty: true}
  networkInterfaces:
    - {ip: 192.0.2.10/24, gateway: 192.0.2.1, bridge: br0}
`

func TestValidateInputDisks(t *testing.T) {
	valid := inputDiskBase + `  inputDisks:
    - label: NODE_CFG1
      files:
        - {name: node.yaml, string: "a: b"}
        - {name: install-config, file: /srv/install.conf}
        - {name: a_b.json, string: "{}"}
    - label: CIDATA
      files:
        - {name: user-data, string: "#cloud-config"}
`
	if _, err := LoadFromYAML([]byte(valid)); err != nil {
		t.Fatalf("valid input disks refused: %v", err)
	}

	disk := func(label, files string) string {
		return inputDiskBase + "  inputDisks:\n    - label: " + label + "\n      files:\n" + files
	}
	file := func(name string) string { return "        - {name: \"" + name + "\", string: x}\n" }
	tests := []struct {
		name, yaml, want string
	}{
		{"empty label", disk(`""`, file("a")), "spec.inputDisks[0].label"},
		{"lower-case label", disk("nodecfg", file("a")), "spec.inputDisks[0].label"},
		{"label with dash", disk("NODE-CFG", file("a")), "spec.inputDisks[0].label"},
		{"long label", disk(strings.Repeat("A", 33), file("a")), "spec.inputDisks[0].label"},
		{"no files", inputDiskBase + "  inputDisks:\n    - label: A\n", "spec.inputDisks[0].files must list"},
		{"upper-case name", disk("A", file("Node.yaml")), "spec.inputDisks[0].files[0].name"},
		{"two dots", disk("A", file("a.b.c")), "files[0].name"},
		{"long extension", disk("A", file("a.ignition1")), "files[0].name"},
		{"subdirectory", disk("A", file("dir/a")), "files[0].name"},
		{"parent directory", disk("A", file("..")), "files[0].name"},
		{"leading dot", disk("A", file(".hidden")), "files[0].name"},
		{"too long", disk("A", file(strings.Repeat("a", 24)+".yaml")), "files[0].name"},
		{"duplicate name", disk("A", file("a")+file("a")), "files[1].name \"a\" is duplicated"},
		{"neither file nor string", disk("A", "        - {name: a}\n"), "exactly one of file or string"},
		{"both file and string", disk("A", "        - {name: a, file: /x, string: y}\n"), "exactly one of file or string"},
		{"duplicate label", inputDiskBase + "  inputDisks:\n    - label: A\n      files:\n" + file("a") + "    - label: A\n      files:\n" + file("a"),
			"spec.inputDisks[1].label \"A\" is already used by spec.inputDisks[0]"},
		{"data disk on an input target", disk("A", file("a")) + "  dataDisks:\n    - {device: sdb, sizeGB: 1}\n",
			"spec.dataDisks[0].device \"sdb\" is the target of spec.inputDisks[0]"},
		{"CIDATA with cloudInit", disk("CIDATA", file("a")) + "  cloudInit: {fqdn: a.example.com}\n", "label CIDATA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromYAML([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}

	// 28 characters is the longest name
	if _, err := LoadFromYAML([]byte(disk("A", file(strings.Repeat("a", 23)+".yaml")))); err != nil {
		t.Errorf("28-character name refused: %v", err)
	}

	many := func(n int) string {
		y := inputDiskBase + "  inputDisks:\n"
		for i := 0; i < n; i++ {
			y += fmt.Sprintf("    - label: D%d\n      files:\n", i) + file("a")
		}
		return y
	}
	if _, err := LoadFromYAML([]byte(many(25))); err != nil {
		t.Errorf("25 input disks refused: %v", err)
	}
	if _, err := LoadFromYAML([]byte(many(26))); err == nil || !strings.Contains(err.Error(), "at most 25") {
		t.Errorf("26 input disks: %v", err)
	}
}

// Inline content is stored with the spec in the domain's metadata, so its
// total is capped; content in files is not.
func TestValidateInputDisks_InlineLimit(t *testing.T) {
	inline := func(n int) string {
		return inputDiskBase + "  inputDisks:\n    - label: A\n      files:\n" +
			"        - {name: a, string: " + strings.Repeat("x", n/2) + "}\n" +
			"        - {name: b, string: " + strings.Repeat("x", n-n/2) + "}\n" +
			"        - {name: c, file: /srv/big.img}\n"
	}
	if _, err := LoadFromYAML([]byte(inline(maxInputStringBytes))); err != nil {
		t.Errorf("1 MiB of inline content refused: %v", err)
	}
	if _, err := LoadFromYAML([]byte(inline(maxInputStringBytes + 1))); err == nil || !strings.Contains(err.Error(), "put larger content in a file") {
		t.Errorf("inline content over 1 MiB: %v", err)
	}
}

// LoadFromFile takes relative input file paths from the YAML file's directory.
func TestLoadFromFile_InputFilePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vm.yaml")
	yaml := inputDiskBase + `  inputDisks:
    - label: A
      files:
        - {name: rel, file: inputs/node.yaml}
        - {name: abs, file: /srv/node.yaml}
        - {name: inline, string: x}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	vm, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := vm.Spec.InputDisks[0].Files
	if files[0].File != filepath.Join(dir, "inputs", "node.yaml") || files[1].File != "/srv/node.yaml" || files[2].File != "" {
		t.Errorf("files %+v", files)
	}
}
