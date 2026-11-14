package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// execute runs the CLI with args, as main does but without exiting, and
// returns its error and what cobra itself wrote (main prints the error).
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	// Another test may have run the pre-run on these commands already
	createCmd.SilenceUsage = false
	logsCmd.SilenceUsage = false
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

// TestErrorsPrintedOnce: cobra prints no error of its own (main prints it,
// once); a mistake in the arguments or flags shows the usage text, an error
// of the command itself does not.
func TestErrorsPrintedOnce(t *testing.T) {
	t.Run("wrong arguments", func(t *testing.T) {
		out, err := execute(t, "create")
		if err == nil {
			t.Fatal("create without a file succeeded")
		}
		if strings.Contains(out, "Error:") {
			t.Errorf("cobra printed the error:\n%s", out)
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("no usage text for an argument mistake:\n%s", out)
		}
	})
	t.Run("unknown flag", func(t *testing.T) {
		out, err := execute(t, "logs", "web", "--no-such-flag")
		if err == nil || strings.Contains(out, "Error:") || !strings.Contains(out, "Usage:") {
			t.Errorf("err %v, output:\n%s", err, out)
		}
	})
	t.Run("command fails", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.yaml")
		out, err := execute(t, "create", missing)
		if err == nil || !strings.Contains(err.Error(), "missing.yaml") {
			t.Fatalf("err = %v, want one naming the missing file", err)
		}
		if strings.Contains(out, "Error:") || strings.Contains(out, "Usage:") {
			t.Errorf("a failing command printed its error or usage through cobra:\n%s", out)
		}
	})
}
