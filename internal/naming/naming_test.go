package naming

import "testing"

func TestMACFromIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name: "basic IP",
			ip:   "10.20.30.40",
			want: "be:ef:0a:14:1e:28",
		},
		{
			name: "IP with CIDR",
			ip:   "10.250.250.10/24",
			want: "be:ef:0a:fa:fa:0a",
		},
		{
			name: "zero octets",
			ip:   "10.0.0.1",
			want: "be:ef:0a:00:00:01",
		},
		{
			name:    "invalid IP",
			ip:      "not-an-ip",
			wantErr: true,
		},
		{
			name:    "IPv6 address",
			ip:      "2001:db8::1",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			ip:      "10.1.2.3/99",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MACFromIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("MACFromIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("MACFromIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInterfaceNameFromIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name: "basic IP",
			ip:   "10.20.30.40",
			want: "vm0a141e28",
		},
		{
			name: "IP with CIDR",
			ip:   "10.250.250.10/24",
			want: "vm0afafa0a",
		},
		{
			name: "high octets",
			ip:   "192.168.1.100",
			want: "vmc0a80164",
		},
		{
			name:    "invalid IP",
			ip:      "not-an-ip",
			wantErr: true,
		},
		{
			name:    "IPv6 address",
			ip:      "2001:db8::1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InterfaceNameFromIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("InterfaceNameFromIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("InterfaceNameFromIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeNameBoot(t *testing.T) {
	tests := []struct {
		vmName string
		want   string
	}{
		{"my-vm", "my-vm_boot.qcow2"},
		{"web-server", "web-server_boot.qcow2"},
		{"vm123", "vm123_boot.qcow2"},
	}

	for _, tt := range tests {
		t.Run(tt.vmName, func(t *testing.T) {
			if got := VolumeNameBoot(tt.vmName); got != tt.want {
				t.Errorf("VolumeNameBoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeNameData(t *testing.T) {
	tests := []struct {
		vmName string
		device string
		want   string
	}{
		{"my-vm", "vdb", "my-vm_data-vdb.qcow2"},
		{"web-server", "vdc", "web-server_data-vdc.qcow2"},
		{"vm123", "vdd", "vm123_data-vdd.qcow2"},
	}

	for _, tt := range tests {
		t.Run(tt.vmName+"_"+tt.device, func(t *testing.T) {
			if got := VolumeNameData(tt.vmName, tt.device); got != tt.want {
				t.Errorf("VolumeNameData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeNameCloudInit(t *testing.T) {
	tests := []struct {
		vmName string
		want   string
	}{
		{"my-vm", "my-vm_cloudinit.iso"},
		{"web-server", "web-server_cloudinit.iso"},
		{"vm123", "vm123_cloudinit.iso"},
	}

	for _, tt := range tests {
		t.Run(tt.vmName, func(t *testing.T) {
			if got := VolumeNameCloudInit(tt.vmName); got != tt.want {
				t.Errorf("VolumeNameCloudInit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeNameInput(t *testing.T) {
	if got := VolumeNameInput("web-server", "NODE_CFG1"); got != "web-server_input-node_cfg1.iso" {
		t.Errorf("VolumeNameInput() = %q", got)
	}
}

// The most input disks validation allows take distinct targets up to sdz.
func TestInputDiskTarget(t *testing.T) {
	seen := map[string]bool{"sda": true}
	for i := 0; i < MaxInputDisks; i++ {
		target := InputDiskTarget(i)
		if seen[target] {
			t.Errorf("target %s of input disk %d is taken", target, i)
		}
		seen[target] = true
	}
	if InputDiskTarget(0) != "sdb" || InputDiskTarget(MaxInputDisks-1) != "sdz" {
		t.Errorf("targets %s .. %s", InputDiskTarget(0), InputDiskTarget(MaxInputDisks-1))
	}
}

func TestVolumeNameVarstore(t *testing.T) {
	if got, want := VolumeNameVarstore("my-keys", "x86_64"), "varstore_my-keys_x86_64.fd"; got != want {
		t.Errorf("VolumeNameVarstore() = %q, want %q", got, want)
	}
}

func TestParseVolumeNameVarstore(t *testing.T) {
	tests := []struct {
		volume      string
		wantProfile string
		wantArch    string
		wantOK      bool
	}{
		{"varstore_my-keys_x86_64.fd", "my-keys", "x86_64", true},
		{"varstore_k_aarch64.fd", "k", "aarch64", true},
		{"fedora-43.qcow2", "", "", false},
		{"varstore_my-keys_x86_64.raw", "", "", false},
		{"varstore_my-keys.fd", "", "", false},
		{"varstore__x86_64.fd", "", "", false},
		{"varstore_k_.fd", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.volume, func(t *testing.T) {
			profile, arch, ok := ParseVolumeNameVarstore(tt.volume)
			if profile != tt.wantProfile || arch != tt.wantArch || ok != tt.wantOK {
				t.Errorf("ParseVolumeNameVarstore(%q) = %q, %q, %v; want %q, %q, %v",
					tt.volume, profile, arch, ok, tt.wantProfile, tt.wantArch, tt.wantOK)
			}
			if ok && VolumeNameVarstore(profile, arch) != tt.volume {
				t.Errorf("round trip of %q failed", tt.volume)
			}
		})
	}
}
