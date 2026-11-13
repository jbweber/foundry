package main

import (
	"testing"

	"github.com/jbweber/foundry/internal/libvirt"
)

// TestSocketFlag: --socket sets the socket every command connects to;
// without it the default choice stays.
func TestSocketFlag(t *testing.T) {
	t.Cleanup(func() {
		socketPath = ""
		libvirt.SetSocket("")
	})
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"list", "--socket", "/tmp/vq.sock"}, "/tmp/vq.sock"},
		{[]string{"--socket=/tmp/other.sock", "pool", "list"}, "/tmp/other.sock"},
	} {
		cmd, rest, err := rootCmd.Find(tt.args)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.ParseFlags(rest); err != nil {
			t.Fatal(err)
		}
		rootCmd.PersistentPreRun(cmd, nil)
		if got := libvirt.Socket(); got != tt.want {
			t.Errorf("%v: socket %q, want %q", tt.args, got, tt.want)
		}
	}
	socketPath = ""
	rootCmd.PersistentPreRun(rootCmd, nil)
	if got := libvirt.Socket(); got != libvirt.QEMUSocket && got != libvirt.LegacySocket {
		t.Errorf("without --socket: %q", got)
	}
}
