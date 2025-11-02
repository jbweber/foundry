package libvirt

import (
	"context"
	"testing"
	"time"
)

// TestConnect tests basic connection functionality.
// This is an integration test that requires libvirt to be running.
func TestConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Try to connect with defaults
	c, err := Connect("", 0)
	if err != nil {
		t.Skipf("libvirt not available: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Verify connection works
	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestConnect_CustomSocket tests connection with a custom socket path.
func TestConnect_CustomSocket(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Try explicit socket path
	c, err := Connect("/var/run/libvirt/libvirt-sock", 5*time.Second)
	if err != nil {
		t.Skipf("libvirt not available: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestConnect_InvalidSocket tests connection failure with invalid socket.
func TestConnect_InvalidSocket(t *testing.T) {
	_, err := Connect("/nonexistent/socket", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error connecting to nonexistent socket, got nil")
	}
}

// TestConnectWithContext_Cancellation tests context cancellation.
func TestConnectWithContext_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	_, err := ConnectWithContext(ctx, "", 0)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// TestConnectWithContext_Success tests successful connection with context.
func TestConnectWithContext_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := ConnectWithContext(ctx, "", 0)
	if err != nil {
		t.Skipf("libvirt not available: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestClose_Idempotent tests that Close can be called multiple times safely.
func TestClose_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c, err := Connect("", 0)
	if err != nil {
		t.Skipf("libvirt not available: %v", err)
	}

	// Close multiple times
	if err := c.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

// TestPing_Disconnected tests Ping on a disconnected client.
func TestPing_Disconnected(t *testing.T) {
	c := &Client{libvirt: nil}

	err := c.Ping()
	if err == nil {
		t.Fatal("expected error from Ping on nil client, got nil")
	}
}

// TestLibvirt_Accessor tests the Libvirt() accessor method.
func TestLibvirt_Accessor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c, err := Connect("", 0)
	if err != nil {
		t.Skipf("libvirt not available: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	l := c.Libvirt()
	if l == nil {
		t.Fatal("Libvirt() returned nil")
	}

	// Verify we can call libvirt APIs directly
	version, err := l.ConnectGetLibVersion()
	if err != nil {
		t.Fatalf("ConnectGetLibVersion failed: %v", err)
	}

	if version == 0 {
		t.Fatal("got version 0, expected non-zero")
	}
}
