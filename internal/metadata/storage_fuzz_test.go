package metadata

import (
	"testing"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/api/v1alpha1"
)

// FuzzStoreLoad stores a spec whose free-text fields hold arbitrary strings
// (user data is a shell script, an input file or fw_cfg string any file, an
// annotation anything) and loads it back: whatever the strings hold, the
// metadata must be valid XML and must read back unchanged. The mock hands back exactly the XML that was stored.
func FuzzStoreLoad(f *testing.F) {
	f.Add("#!/bin/sh\ntrue && echo '<ok>' > /dev/null\n", "a & b")
	f.Add("]]><x/>", "\r\n\t")
	f.Add("\x00\x1b[0m\xff", "<!-- -->")
	f.Add("]]><C/>", "\t\n0") // go-yaml's Marshal writes a block it cannot read back
	f.Add("0", "\n#")         // a block with an indentation indicator, read back as ""
	f.Fuzz(func(t *testing.T, text, note string) {
		// Each string is stored up to three times and escaped (up to six
		// bytes a character); stay clear of MaxRecordBytes, which
		// TestEncode_Bound covers
		if 18*(len(text)+len(note)) > MaxRecordBytes {
			t.Skip("inputs exceed the record bound")
		}
		vm := newTestVM("fuzz")
		vm.Annotations = map[string]string{"note": note}
		vm.Spec.CloudInit = &v1alpha1.CloudInitSpec{RawUserData: text}
		vm.Spec.FwCfg = []v1alpha1.FwCfgEntrySpec{{Name: "opt/x", String: text}}
		vm.Spec.InputDisks = []v1alpha1.InputDiskSpec{{Label: "CFG", Files: []v1alpha1.InputFileSpec{{Name: "a", String: note}}}}

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
		if got.Spec.CloudInit == nil || got.Spec.CloudInit.RawUserData != text {
			t.Fatalf("user data %q does not read back unchanged: %+v", text, got.Spec.CloudInit)
		}
		if len(got.Spec.FwCfg) != 1 || got.Spec.FwCfg[0].String != text {
			t.Fatalf("fw_cfg string %q does not read back unchanged: %+v", text, got.Spec.FwCfg)
		}
		if len(got.Spec.InputDisks) != 1 || len(got.Spec.InputDisks[0].Files) != 1 || got.Spec.InputDisks[0].Files[0].String != note {
			t.Fatalf("input file %q does not read back unchanged: %+v", note, got.Spec.InputDisks)
		}
		if got.Annotations["note"] != note {
			t.Fatalf("annotation %q reads back as %q", note, got.Annotations["note"])
		}
	})
}
