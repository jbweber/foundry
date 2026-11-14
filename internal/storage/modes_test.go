package storage

import (
	"strings"
	"testing"
)

// TestGenerateVolumeXML_Modes: a VM's own volumes are created 0600, base
// images and key profiles 0644, all without an owner or group, which libvirt
// sets itself while a domain uses the volume.
func TestGenerateVolumeXML_Modes(t *testing.T) {
	for _, tt := range []struct {
		typ  VolumeType
		mode string
	}{
		{VolumeTypeBoot, "0600"},
		{VolumeTypeData, "0600"},
		{VolumeTypeCloudInit, "0600"},
		{VolumeTypeInput, "0600"},
		{VolumeTypeBaseImage, "0644"},
		{VolumeTypeVarstore, "0644"},
	} {
		t.Run(string(tt.typ), func(t *testing.T) {
			xml, err := generateVolumeXML("pool", VolumeSpec{Name: "v", Type: tt.typ, Format: VolumeFormatRaw, CapacityBytes: 1}, nil)
			if err != nil {
				t.Fatalf("generateVolumeXML: %v", err)
			}
			if !strings.Contains(xml, "<mode>"+tt.mode+"</mode>") {
				t.Errorf("want mode %s:\n%s", tt.mode, xml)
			}
			if strings.Contains(xml, "<owner>") || strings.Contains(xml, "<group>") {
				t.Errorf("volume XML sets an owner or group:\n%s", xml)
			}
		})
	}
}

// TestGenerateDirPoolXML_Modes: the VMs pool's directory is 0711, others
// 0755, without an owner or group.
func TestGenerateDirPoolXML_Modes(t *testing.T) {
	for pool, mode := range map[string]string{DefaultVMsPool: "0711", DefaultImagesPool: "0755", "fast": "0755"} {
		xml, err := generateDirPoolXML(pool, "/var/lib/foundry/"+pool)
		if err != nil {
			t.Fatalf("generateDirPoolXML(%s): %v", pool, err)
		}
		if !strings.Contains(xml, "<mode>"+mode+"</mode>") {
			t.Errorf("pool %s: want mode %s:\n%s", pool, mode, xml)
		}
		if strings.Contains(xml, "<owner>") || strings.Contains(xml, "<group>") {
			t.Errorf("pool %s XML sets an owner or group:\n%s", pool, xml)
		}
	}
}
