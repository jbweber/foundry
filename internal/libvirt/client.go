package libvirt

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
)

// Client wraps a go-libvirt connection and provides high-level operations
// for managing VMs.
type Client struct {
	libvirt *libvirt.Libvirt
}

// Connect establishes a connection to the local libvirt daemon.
// It returns a Client that must be closed via Close() when done.
//
// If socketPath is empty, defaults to "/var/run/libvirt/libvirt-sock" (qemu:///system)
// If timeout is zero, defaults to 5 seconds.
//
// This matches the Ansible implementation which uses the default
// local qemu:///system connection (UNIX domain socket).
func Connect(socketPath string, timeout time.Duration) (*Client, error) {
	// Set defaults
	if socketPath == "" {
		socketPath = "/var/run/libvirt/libvirt-sock"
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// Create local dialer with options
	dialer := dialers.NewLocal(
		dialers.WithSocket(socketPath),
		dialers.WithLocalTimeout(timeout),
	)

	// Create libvirt client and connect
	l := libvirt.NewWithDialer(dialer)
	if err := l.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt at %s: %w", socketPath, err)
	}

	return &Client{libvirt: l}, nil
}

// ConnectWithContext establishes a connection with context support for cancellation.
func ConnectWithContext(ctx context.Context, socketPath string, timeout time.Duration) (*Client, error) {
	// Create a channel for the connection result
	type result struct {
		client *Client
		err    error
	}
	resultCh := make(chan result, 1)

	// Attempt connection in a goroutine
	go func() {
		c, err := Connect(socketPath, timeout)
		resultCh <- result{client: c, err: err}
	}()

	// Wait for either context cancellation or connection completion
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("connection cancelled: %w", ctx.Err())
	case res := <-resultCh:
		return res.client, res.err
	}
}

// Close closes the libvirt connection and releases resources.
// It is safe to call Close multiple times.
func (c *Client) Close() error {
	if c.libvirt == nil {
		return nil
	}

	if err := c.libvirt.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect from libvirt: %w", err)
	}

	return nil
}

// Libvirt returns the underlying go-libvirt client for direct API access.
// This should be used sparingly; prefer higher-level methods on Client.
func (c *Client) Libvirt() *libvirt.Libvirt {
	return c.libvirt
}

// Ping verifies the connection is still alive by calling a simple libvirt API.
func (c *Client) Ping() error {
	if c.libvirt == nil {
		return fmt.Errorf("client not connected")
	}

	// Try to get libvirt version as a ping test
	_, err := c.libvirt.ConnectGetLibVersion()
	if err != nil {
		return fmt.Errorf("libvirt connection is dead: %w", err)
	}

	return nil
}
