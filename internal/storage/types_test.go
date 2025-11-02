package storage

import (
	"testing"
)

func TestVolumeSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    VolumeSpec
		wantErr bool
	}{
		{
			name: "valid boot disk spec",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			wantErr: false,
		},
		{
			name: "valid boot disk with backing volume",
			spec: VolumeSpec{
				Name:          "my-vm_boot",
				Type:          VolumeTypeBoot,
				Format:        VolumeFormatQCOW2,
				CapacityGB:    50,
				BackingVolume: "fedora-43",
			},
			wantErr: false,
		},
		{
			name: "valid data disk spec",
			spec: VolumeSpec{
				Name:       "my-vm_data-vdb",
				Type:       VolumeTypeData,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 100,
			},
			wantErr: false,
		},
		{
			name: "valid cloud-init ISO spec",
			spec: VolumeSpec{
				Name:   "my-vm_cloudinit",
				Type:   VolumeTypeCloudInit,
				Format: VolumeFormatRaw,
			},
			wantErr: false,
		},
		{
			name: "valid raw format",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatRaw,
				CapacityGB: 50,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			spec: VolumeSpec{
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			wantErr: true,
		},
		{
			name: "missing type",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Format:     VolumeFormatQCOW2,
				CapacityGB: 50,
			},
			wantErr: true,
		},
		{
			name: "missing format",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				CapacityGB: 50,
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     "invalid",
				CapacityGB: 50,
			},
			wantErr: true,
		},
		{
			name: "zero capacity for boot disk",
			spec: VolumeSpec{
				Name:       "my-vm_boot",
				Type:       VolumeTypeBoot,
				Format:     VolumeFormatQCOW2,
				CapacityGB: 0,
			},
			wantErr: true,
		},
		{
			name: "backing volume with raw format",
			spec: VolumeSpec{
				Name:          "my-vm_boot",
				Type:          VolumeTypeBoot,
				Format:        VolumeFormatRaw,
				CapacityGB:    50,
				BackingVolume: "fedora-43",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("VolumeSpec.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPoolInfo_Conversions(t *testing.T) {
	info := PoolInfo{
		Capacity:   100 * 1024 * 1024 * 1024, // 100 GB
		Allocation: 50 * 1024 * 1024 * 1024,  // 50 GB
		Available:  50 * 1024 * 1024 * 1024,  // 50 GB
	}

	if got := info.CapacityGB(); got != 100.0 {
		t.Errorf("CapacityGB() = %v, want 100.0", got)
	}

	if got := info.AllocationGB(); got != 50.0 {
		t.Errorf("AllocationGB() = %v, want 50.0", got)
	}

	if got := info.AvailableGB(); got != 50.0 {
		t.Errorf("AvailableGB() = %v, want 50.0", got)
	}
}

func TestVolumeInfo_Conversions(t *testing.T) {
	info := VolumeInfo{
		Capacity:   50 * 1024 * 1024 * 1024, // 50 GB
		Allocation: 25 * 1024 * 1024 * 1024, // 25 GB
	}

	if got := info.CapacityGB(); got != 50.0 {
		t.Errorf("CapacityGB() = %v, want 50.0", got)
	}

	if got := info.AllocationGB(); got != 25.0 {
		t.Errorf("AllocationGB() = %v, want 25.0", got)
	}
}
