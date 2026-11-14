package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// FuzzLoadFromYAML feeds the spec loader arbitrary bytes: a VM file is
// input foundry did not write. The loader must never panic, and a spec it
// accepts must survive a round trip: written out as YAML as create stores it
// in the domain's metadata (v1alpha1.EncodeYAML) and loaded again, it is accepted again with
// the same name, so what foundry stores is something it can read back.
func FuzzLoadFromYAML(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range seeds {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata: {name: fuzz}
spec:
  vcpus: 1
  memoryGiB: 1
  bootDisk: {sizeGB: 1, empty: true}
  cloudInit: {rawUserData: "a && b < c"}
  networkInterfaces: [{ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0, vlanTrunk: [10, 20]}]
`))
	f.Add([]byte(`{"apiVersion":"foundry.cofront.xyz/v1alpha1","kind":"VirtualMachine","metadata":{"name":"j"}}`))
	f.Add([]byte("{"))
	f.Add([]byte("apiVersion: foundry.cofront.xyz/v1alpha1\nkind: VirtualMachine\nmetadata: {name: t}\nspec: {vcpus: 1, memoryGiB: 1, bootDisk: {sizeGB: 1, empty: true}, networkInterfaces: [{ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0}], cloudInit: {rawUserData: \"\\tindented\\nscript\"}}\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		vm, err := LoadFromYAML(data)
		if err != nil {
			return
		}
		out, err := v1alpha1.EncodeYAML(vm)
		if err != nil {
			t.Fatalf("an accepted spec does not marshal: %v", err)
		}
		again, err := LoadFromYAML(out)
		if err != nil {
			t.Fatalf("an accepted spec is refused after a round trip: %v\n%s", err, out)
		}
		if again.Name != vm.Name {
			t.Fatalf("name changed in a round trip: %q -> %q", vm.Name, again.Name)
		}
	})
}
