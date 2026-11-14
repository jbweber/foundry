package loader

import (
	"strings"
	"testing"
)

// TestLoadFromYAML_ImagePool: a boot image comes from foundry-images only;
// another pool, in imagePool or the pool:volume form, is refused when the
// spec is loaded. Create takes the image from foundry-images, so another
// pool would boot another image than the one meant.
func TestLoadFromYAML_ImagePool(t *testing.T) {
	base := strings.Replace(dataDeviceBase, "    image: fedora-43.qcow2\n", "", 1)
	for _, tt := range []struct{ name, bootDisk, wantErr string }{
		{"volume name", "    image: fedora-43.qcow2\n", ""},
		{"imagePool foundry-images", "    image: fedora-43.qcow2\n    imagePool: foundry-images\n", ""},
		{"pool:volume foundry-images", "    image: \"foundry-images:fedora-43.qcow2\"\n", ""},
		{"host path", "    image: /srv/images/fedora.qcow2\n", ""},
		{"another imagePool", "    image: fedora-43.qcow2\n    imagePool: fast\n", `spec.bootDisk.imagePool names pool "fast"`},
		{"another pool by reference", "    image: \"other-pool:fedora.qcow2\"\n", `spec.bootDisk.image names pool "other-pool"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := strings.Replace(base, "    sizeGB: 50\n", "    sizeGB: 50\n"+tt.bootDisk, 1)
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
