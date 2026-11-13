package libvirt

import (
	"errors"
	"strings"
	"testing"
)

type fakeCaps struct {
	xml string
	err error
}

func (f fakeCaps) ConnectGetCapabilities() (string, error) { return f.xml, f.err }

func TestHostArch(t *testing.T) {
	tests := []struct {
		name    string
		caps    fakeCaps
		want    string
		wantErr string
	}{
		{
			name: "x86_64",
			caps: fakeCaps{xml: `<capabilities><host><cpu><arch>x86_64</arch></cpu></host></capabilities>`},
			want: "x86_64",
		},
		{
			name: "aarch64",
			caps: fakeCaps{xml: `<capabilities><host><cpu><arch>aarch64</arch></cpu></host></capabilities>`},
			want: "aarch64",
		},
		{
			name:    "no arch",
			caps:    fakeCaps{xml: `<capabilities><host></host></capabilities>`},
			wantErr: "no architecture",
		},
		{
			name:    "unparsable",
			caps:    fakeCaps{xml: `<capabilities`},
			wantErr: "failed to parse",
		},
		{
			name:    "read error",
			caps:    fakeCaps{err: errors.New("boom")},
			wantErr: "failed to read host capabilities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HostArch(tt.caps)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("HostArch() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("HostArch() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("HostArch() = %s, want %s", got, tt.want)
			}
		})
	}
}
