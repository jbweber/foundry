package metadata

import (
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// FuzzStoreLoad stores a spec whose free-text fields hold arbitrary strings
// (user data is a shell script, an annotation anything) and loads it back:
// whatever the strings hold, the metadata must be valid XML and must read
// back unchanged. The mock hands back exactly the XML that was stored.
func FuzzStoreLoad(f *testing.F) {
	f.Add("#!/bin/sh\ntrue && echo '<ok>' > /dev/null\n", "a & b")
	f.Add("]]><x/>", "\r\n\t")
	f.Add("\x00\x1b[0m\xff", "<!-- -->")
	f.Add("]]><C/>", "\t\n0") // go-yaml's Marshal writes a block it cannot read back
	f.Fuzz(func(t *testing.T, userData, note string) {
		vm := newTestVM("fuzz")
		vm.Annotations = map[string]string{"note": note}
		vm.Spec.CloudInit = &v1alpha1.CloudInitSpec{RawUserData: userData}

		mock := &mockLibvirtClient{}
		client := NewClient(mock)
		if err := client.Store(libvirt.Domain{}, vm); err != nil {
			t.Fatalf("Store: %v", err)
		}
		mock.getMetadataValue = mock.lastSetMetadata
		got, err := client.Load(libvirt.Domain{})
		if err != nil {
			t.Fatalf("Load: %v\n%s", err, mock.lastSetMetadata)
		}
		if got.Spec.CloudInit == nil || got.Spec.CloudInit.RawUserData != userData {
			t.Fatalf("user data %q does not read back unchanged: %+v", userData, got.Spec.CloudInit)
		}
		if got.Annotations["note"] != note {
			t.Fatalf("annotation %q reads back as %q", note, got.Annotations["note"])
		}
	})
}
