package loader

import (
	"fmt"
	"strings"
	"testing"
)

// A spec's image digest has one form, and goes with an image volume in
// foundry-images, the one create boots from.
func TestLoadFromYAML_ImageSHA256(t *testing.T) {
	good := strings.Repeat("a", 64)
	spec := func(bootDisk string) string {
		return fmt.Sprintf(`apiVersion: foundry.cofront.xyz/v1alpha1
kind: VirtualMachine
metadata: {name: vm1}
spec:
  vcpus: 1
  memoryGiB: 1
  bootDisk: {sizeGB: 10, %s}
  networkInterfaces:
    - {ip: 10.0.0.2/24, gateway: 10.0.0.1, bridge: br0}
`, bootDisk)
	}
	for _, tt := range []struct {
		name, bootDisk, want string
	}{
		{"none", "image: a.qcow2", ""},
		{"image", "image: a.qcow2, imageSHA256: " + good, ""},
		{"pool and volume", "image: \"foundry-images:a.qcow2\", imageSHA256: " + good, ""},
		{"imagePool foundry-images", "image: a.qcow2, imagePool: foundry-images, imageSHA256: " + good, ""},
		{"short", "image: a.qcow2, imageSHA256: abc", "imageSHA256"},
		{"65 digits", "image: a.qcow2, imageSHA256: " + good + "a", "imageSHA256"},
		{"upper case", "image: a.qcow2, imageSHA256: " + strings.Repeat("A", 64), "imageSHA256"},
		{"not hex", "image: a.qcow2, imageSHA256: " + strings.Repeat("g", 64), "imageSHA256"},
		{"sha256: prefix", "image: a.qcow2, imageSHA256: \"sha256:" + good + "\"", "imageSHA256"},
		{"a file path", "image: /x/a.qcow2, imageSHA256: " + good, "file path"},
		{"a relative path", "image: ./a.qcow2, imageSHA256: " + good, "file path"},
		{"empty disk", "empty: true, imageSHA256: " + good, "empty disk"},
		{"another pool by reference", "image: \"other:a.qcow2\", imageSHA256: " + good, "pool \"other\""},
		{"another imagePool", "image: a.qcow2, imagePool: other, imageSHA256: " + good, "pool \"other\""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vm, err := LoadFromYAML([]byte(spec(tt.bootDisk)))
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("refused: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("err = %v, want one about %s", err, tt.want)
			case tt.want == "" && strings.Contains(tt.bootDisk, good) && vm.Spec.BootDisk.ImageSHA256 != good:
				t.Errorf("imageSHA256 = %q, want %q", vm.Spec.BootDisk.ImageSHA256, good)
			}
		})
	}
}
