package libvirt

import (
	"strings"
	"testing"
	"time"
)

// TestChooseSocket: virtqemud's socket when it exists, else libvirt-sock.
func TestChooseSocket(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]bool
		want     string
	}{
		{"modular daemons", map[string]bool{QEMUSocket: true, LegacySocket: true}, QEMUSocket},
		{"only virtqemud", map[string]bool{QEMUSocket: true}, QEMUSocket},
		{"monolithic libvirtd", map[string]bool{LegacySocket: true}, LegacySocket},
		{"nothing running", map[string]bool{}, LegacySocket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := func(path string) bool { return tt.existing[path] }
			if got := chooseSocket(exists); got != tt.want {
				t.Errorf("chooseSocket() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSetSocket: a set socket replaces the default choice; clearing it
// restores the default.
func TestSetSocket(t *testing.T) {
	t.Cleanup(func() { SetSocket("") })

	SetSocket("/tmp/forwarded.sock")
	if got := Socket(); got != "/tmp/forwarded.sock" {
		t.Errorf("Socket() = %q, want the set socket", got)
	}

	SetSocket("")
	if got := Socket(); got != DefaultSocket() {
		t.Errorf("Socket() = %q, want the default %q", got, DefaultSocket())
	}
}

// TestConnect_SetSocketUsed: a connection given no socket goes to the set one.
func TestConnect_SetSocketUsed(t *testing.T) {
	t.Cleanup(func() { SetSocket("") })
	missing := t.TempDir() + "/no-such-sock"
	SetSocket(missing)

	_, err := Connect("", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error connecting to a missing socket")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name the set socket %s", err, missing)
	}
}

// TestConnect_ExplicitSocketWins: an explicit socket path is used as given,
// even when another socket is set.
func TestConnect_ExplicitSocketWins(t *testing.T) {
	t.Cleanup(func() { SetSocket("") })
	SetSocket("/tmp/other.sock")
	missing := t.TempDir() + "/no-such-sock"

	_, err := Connect(missing, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error connecting to a missing socket")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name the explicit socket %s", err, missing)
	}
}
