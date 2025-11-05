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
