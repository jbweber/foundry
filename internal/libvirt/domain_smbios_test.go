package libvirt

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func sysInfoEntries(entries []libvirtxml.DomainSysInfoEntry) map[string]string {
	m := make(map[string]string)
	for _, e := range entries {
		m[e.Name] = e.Value
	}
	return m
}

func TestGenerateDomainXML_NoSMBIOSOrFwCfg(t *testing.T) {
	for _, arch := range []string{"", v1alpha1.ArchAarch64} {
		vm := archTestVM(arch)
		xml, err := GenerateDomainXML(vm, arch)
		if err != nil {
			t.Fatalf("GenerateDomainXML() error = %v", err)
		}
		if strings.Contains(xml, "sysinfo") || strings.Contains(xml, "smbios") {
			t.Errorf("arch %q: unexpected sysinfo or smbios without spec.serial, spec.smbios or spec.fwCfg:\n%s", arch, xml)
		}
		d := renderDomain(t, vm, arch)
		if d.UUID != "" {
			t.Errorf("arch %q: UUID = %q, want unset", arch, d.UUID)
		}
	}
}

func TestGenerateDomainXML_SMBIOS(t *testing.T) {
	const uuid = "4c4c4544-0042-3510-8051-b4c04f4e3532"
	tests := []struct {
		name     string
		serial   string
		smbios   *v1alpha1.SMBIOSSpec
		want     map[string]string
		wantUUID string
	}{
		{
			name:   "empty smbios gives the default serial",
			smbios: &v1alpha1.SMBIOSSpec{},
			want:   map[string]string{"serial": "foundry-test-vm"},
		},
		{
			name:   "serial only",
			serial: "SN-0001",
			want:   map[string]string{"serial": "SN-0001"},
		},
		{
			name:   "all fields",
			serial: "SN-0001",
			smbios: &v1alpha1.SMBIOSSpec{
				Manufacturer: "Example Corp",
				Product:      "Model 9 (rev A)",
				Version:      "1.0",
				SKU:          "SKU/42",
				Family:       "Servers",
				UUID:         uuid,
			},
			want: map[string]string{
				"manufacturer": "Example Corp",
				"product":      "Model 9 (rev A)",
				"version":      "1.0",
				"serial":       "SN-0001",
				"uuid":         uuid,
				"sku":          "SKU/42",
				"family":       "Servers",
			},
			wantUUID: uuid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := archTestVM("")
			vm.Name = "test-vm"
			vm.Spec.Serial = tt.serial
			vm.Spec.SMBIOS = tt.smbios
			d := renderDomain(t, vm, "")

			if d.OS.SMBios == nil || d.OS.SMBios.Mode != "sysinfo" {
				t.Fatalf("os smbios = %+v, want mode sysinfo", d.OS.SMBios)
			}
			if len(d.SysInfo) != 1 || d.SysInfo[0].SMBIOS == nil || d.SysInfo[0].SMBIOS.System == nil {
				t.Fatalf("sysinfo = %+v, want one smbios block with system entries", d.SysInfo)
			}
			got := sysInfoEntries(d.SysInfo[0].SMBIOS.System.Entry)
			if len(got) != len(tt.want) {
				t.Errorf("system entries = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("system entry %s = %q, want %q", k, got[k], v)
				}
			}
			if d.UUID != tt.wantUUID {
				t.Errorf("domain UUID = %q, want %q", d.UUID, tt.wantUUID)
			}
		})
	}
}

func TestGenerateDomainXML_SMBIOSEntryOrder(t *testing.T) {
	vm := archTestVM("")
	vm.Spec.Serial = "SN-1"
	vm.Spec.SMBIOS = &v1alpha1.SMBIOSSpec{Family: "f", Manufacturer: "m", SKU: "s", Product: "p"}
	d := renderDomain(t, vm, "")
	var names []string
	for _, e := range d.SysInfo[0].SMBIOS.System.Entry {
		names = append(names, e.Name)
	}
	if got, want := strings.Join(names, ","), "manufacturer,product,serial,sku,family"; got != want {
		t.Errorf("entry order = %s, want %s", got, want)
	}
}

func TestGenerateDomainXML_FwCfg(t *testing.T) {
	for _, arch := range []string{"", v1alpha1.ArchAarch64} {
		vm := archTestVM(arch)
		vm.Spec.FwCfg = []v1alpha1.FwCfgEntrySpec{
			{Name: "opt/org.tianocore/EnableLegacyLoader", String: "no"},
			{Name: "opt/com.example/config", File: "/srv/config.json"},
		}
		d := renderDomain(t, vm, arch)
		if d.OS.SMBios != nil {
			t.Errorf("arch %q: fw_cfg alone set os smbios = %+v", arch, d.OS.SMBios)
		}
		if len(d.SysInfo) != 1 || d.SysInfo[0].FWCfg == nil {
			t.Fatalf("arch %q: sysinfo = %+v, want one fwcfg block", arch, d.SysInfo)
		}
		e := d.SysInfo[0].FWCfg.Entry
		if len(e) != 2 {
			t.Fatalf("arch %q: fwcfg entries = %+v, want 2", arch, e)
		}
		if e[0].Name != "opt/org.tianocore/EnableLegacyLoader" || e[0].Value != "no" || e[0].File != "" {
			t.Errorf("arch %q: entry 0 = %+v", arch, e[0])
		}
		if e[1].Name != "opt/com.example/config" || e[1].File != "/srv/config.json" || e[1].Value != "" {
			t.Errorf("arch %q: entry 1 = %+v", arch, e[1])
		}
	}
}

func TestGenerateDomainXML_SMBIOSAndFwCfg(t *testing.T) {
	vm := archTestVM("")
	vm.Spec.Serial = "SN-1"
	vm.Spec.FwCfg = []v1alpha1.FwCfgEntrySpec{{Name: "opt/com.example/a", String: "1"}}
	d := renderDomain(t, vm, "")
	if len(d.SysInfo) != 2 || d.SysInfo[0].SMBIOS == nil || d.SysInfo[1].FWCfg == nil {
		t.Fatalf("sysinfo = %+v, want an smbios block then a fwcfg block", d.SysInfo)
	}
}
