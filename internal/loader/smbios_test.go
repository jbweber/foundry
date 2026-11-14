package loader

import (
	"strings"
	"testing"

	"github.com/jbweber/foundry/api/v1alpha1"
)

func smbiosTestVM() *v1alpha1.VirtualMachine {
	return &v1alpha1.VirtualMachine{
		ObjectMeta: v1alpha1.ObjectMeta{Name: "smbios-vm"},
		Spec: v1alpha1.VirtualMachineSpec{
			VCPUs:     2,
			MemoryGiB: 4,
			BootDisk:  v1alpha1.BootDiskSpec{SizeGB: 20, Image: "fedora-43.qcow2"},
			NetworkInterfaces: []v1alpha1.NetworkInterfaceSpec{
				{IP: "10.0.0.1/24", Gateway: "10.0.0.254", Bridge: "br0"},
			},
		},
	}
}

func checkErr(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("validateSpec() error = %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("validateSpec() error = %v, want it to contain %q", err, wantErr)
	}
}

func TestValidateSpec_SMBIOS(t *testing.T) {
	tests := []struct {
		name    string
		vmName  string
		serial  string
		smbios  *v1alpha1.SMBIOSSpec
		wantErr string
	}{
		{name: "neither set"},
		{name: "neither set, name not a valid serial", vmName: "vm+1"},
		{name: "serial", serial: "SN_01.a-b"},
		{name: "serial 64 characters", serial: strings.Repeat("s", 64)},
		{name: "empty smbios", smbios: &v1alpha1.SMBIOSSpec{}},
		{name: "all fields", smbios: &v1alpha1.SMBIOSSpec{
			Manufacturer: "Example Corp", Product: "Model 9 (rev A)", Version: "v1.0+b/2:3",
			SKU: "SKU-42", Family: "Servers", UUID: "4c4c4544-0042-3510-8051-b4c04f4e3532",
		}},
		{name: "serial too long", serial: strings.Repeat("s", 65), wantErr: "spec.serial"},
		{name: "serial with space", serial: "SN 1", wantErr: "spec.serial"},
		{name: "serial with comma", serial: "SN,1", wantErr: "spec.serial"},
		{name: "default serial from bad name", vmName: "vm+1", smbios: &v1alpha1.SMBIOSSpec{}, wantErr: "the default is \"foundry-\""},
		{name: "leading space", smbios: &v1alpha1.SMBIOSSpec{Manufacturer: " Example"}, wantErr: "spec.smbios.manufacturer"},
		{name: "trailing space", smbios: &v1alpha1.SMBIOSSpec{Product: "Model "}, wantErr: "spec.smbios.product"},
		{name: "comma", smbios: &v1alpha1.SMBIOSSpec{Version: "1,2"}, wantErr: "spec.smbios.version"},
		{name: "equals", smbios: &v1alpha1.SMBIOSSpec{SKU: "a=b"}, wantErr: "spec.smbios.sku"},
		{name: "quote", smbios: &v1alpha1.SMBIOSSpec{Family: `"f"`}, wantErr: "spec.smbios.family"},
		{name: "too long", smbios: &v1alpha1.SMBIOSSpec{Family: strings.Repeat("f", 65)}, wantErr: "spec.smbios.family"},
		{name: "bad uuid", smbios: &v1alpha1.SMBIOSSpec{UUID: "not-a-uuid"}, wantErr: "spec.smbios.uuid"},
		{name: "uuid without dashes", smbios: &v1alpha1.SMBIOSSpec{UUID: "4c4c4544004235108051b4c04f4e3532"}, wantErr: "spec.smbios.uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := smbiosTestVM()
			if tt.vmName != "" {
				vm.Name = tt.vmName
			}
			vm.Spec.Serial = tt.serial
			vm.Spec.SMBIOS = tt.smbios
			checkErr(t, validateSpec(vm), tt.wantErr)
		})
	}
}

func TestValidateSpec_FwCfg(t *testing.T) {
	tests := []struct {
		name    string
		entries []v1alpha1.FwCfgEntrySpec
		wantErr string
	}{
		{name: "none"},
		{name: "string and file", entries: []v1alpha1.FwCfgEntrySpec{
			{Name: "opt/org.tianocore/EnableLegacyLoader", String: "no"},
			{Name: "opt/com.example/config", File: "/srv/config.json"},
		}},
		{name: "not under opt", entries: []v1alpha1.FwCfgEntrySpec{{Name: "etc/foo", String: "x"}}, wantErr: `spec.fwCfg[0].name "etc/foo" must start with "opt/"`},
		{name: "reserved ovmf", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/ovmf/X-PciMmio64Mb", String: "1"}}, wantErr: "reserved by libvirt"},
		{name: "reserved qemu", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/org.qemu/x", String: "1"}}, wantErr: "reserved by libvirt"},
		{name: "name too long", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/" + strings.Repeat("x", 52), String: "1"}}, wantErr: "is 56 characters; QEMU allows at most 55"},
		{name: "name at the limit", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/" + strings.Repeat("x", 51), String: "1"}}},
		{name: "neither file nor string", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a"}}, wantErr: "exactly one of file or string"},
		{name: "both file and string", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a", File: "/f", String: "s"}}, wantErr: "exactly one of file or string"},
		{name: "duplicate", entries: []v1alpha1.FwCfgEntrySpec{
			{Name: "opt/a", String: "1"},
			{Name: "opt/a", String: "2"},
		}, wantErr: `spec.fwCfg[1].name "opt/a" is duplicated`},
		// libvirt passes a string entry to QEMU as one argument, which exec
		// takes up to 128 KiB long
		{name: "string at the limit", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a", String: strings.Repeat("x", fwCfgMaxString)}}},
		{name: "string over the limit", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a", String: strings.Repeat("x", fwCfgMaxString+1)}}, wantErr: "spec.fwCfg[0].string is 130945 bytes; at most 130944 fit in the QEMU argument libvirt passes it in, put larger content in a file"},
		// libvirt does not escape commas in a string entry's QEMU argument
		{name: "string without a comma", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a", String: "key=value; a b:c/d"}}},
		{name: "string with a comma", entries: []v1alpha1.FwCfgEntrySpec{{Name: "opt/a", String: "a,b"}}, wantErr: "spec.fwCfg[0].string contains a comma, which libvirt passes to QEMU unescaped, so the VM would fail to start; put the content in a file entry instead"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := smbiosTestVM()
			vm.Spec.FwCfg = tt.entries
			checkErr(t, validateSpec(vm), tt.wantErr)
		})
	}
}

func TestLoadFromYAML_SMBIOSAndFwCfg(t *testing.T) {
	data := []byte(`apiVersion: ` + v1alpha1.GroupName + "/" + v1alpha1.Version + `
kind: VirtualMachine
metadata:
  name: SMBIOS-VM
spec:
  vcpus: 2
  memoryGiB: 4
  bootDisk:
    sizeGB: 20
    image: fedora-43.qcow2
  networkInterfaces:
    - ip: 10.0.0.1/24
      gateway: 10.0.0.254
      bridge: br0
  smbios:
    uuid: 4C4C4544-0042-3510-8051-B4C04F4E3532
  fwCfg:
    - name: opt/org.tianocore/EnableLegacyLoader
      string: "no"
`)
	vm, err := LoadFromYAML(data)
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	if vm.Spec.SMBIOS == nil || vm.Spec.SMBIOS.UUID != "4c4c4544-0042-3510-8051-b4c04f4e3532" {
		t.Errorf("spec.smbios = %+v, want the UUID lower-cased", vm.Spec.SMBIOS)
	}
	if got := vm.GetSerial(); got != "foundry-smbios-vm" {
		t.Errorf("GetSerial() = %q, want foundry-smbios-vm", got)
	}
	if len(vm.Spec.FwCfg) != 1 || vm.Spec.FwCfg[0].String != "no" {
		t.Errorf("spec.fwCfg = %+v", vm.Spec.FwCfg)
	}

	empty := []byte(strings.Replace(string(data), "  smbios:\n    uuid: 4C4C4544-0042-3510-8051-B4C04F4E3532\n", "  smbios: {}\n", 1))
	vm, err = LoadFromYAML(empty)
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	if !vm.HasSMBIOS() {
		t.Errorf("smbios: {} did not turn SMBIOS on")
	}
}
