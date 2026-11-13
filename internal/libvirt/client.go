package libvirt

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"libvirt.org/go/libvirtxml"
)

// Client wraps a go-libvirt connection and provides high-level operations
// for managing VMs.
type Client struct {
	libvirt *libvirt.Libvirt
}

// Local libvirt sockets for qemu:///system.
const (
	// QEMUSocket is virtqemud's socket: the QEMU driver's own daemon on a
	// host that runs libvirt's modular daemons.
	QEMUSocket = "/var/run/libvirt/virtqemud-sock"
	// LegacySocket is the monolithic libvirtd's socket. On a modular-daemon
	// host it is served by virtproxyd, a proxy to the per-driver daemons.
	LegacySocket = "/var/run/libvirt/libvirt-sock"
)

var (
	socketMu       sync.RWMutex
	socketOverride string
)

// SetSocket sets the socket that connections use when they are given none.
// An empty path restores the default choice (see DefaultSocket). The CLI
// calls it once with its --socket flag.
func SetSocket(path string) {
	socketMu.Lock()
	defer socketMu.Unlock()
	socketOverride = path
}

// Socket returns the socket a connection given no socket uses: the one set
// with SetSocket, else DefaultSocket's choice.
func Socket() string {
	socketMu.RLock()
	override := socketOverride
	socketMu.RUnlock()
	if override != "" {
		return override
	}
	return DefaultSocket()
}

// DefaultSocket returns the socket a qemu:///system connection uses when none
// is given, by libvirt's own client rule for local connections: the
// per-driver daemon's socket if it exists, otherwise the legacy one
// (remoteGetUNIXSocket in
// https://gitlab.com/libvirt/libvirt/-/blob/master/src/remote/remote_sockets.c;
// https://libvirt.org/daemons.html).
//
// One connection to virtqemud is enough for everything foundry does: for a
// client whose primary driver is QEMU, virtqemud opens the secondary drivers
// itself (storage, network, ...) and forwards those calls, so storage pool and
// volume calls work on it as on libvirt-sock.
//
// Talking to virtqemud directly rather than through virtproxyd matters under
// SELinux: virtproxyd identifies each client from its /proc entry, and its
// policy may not allow that for a confined caller (a service or a container),
// which then cannot connect at all. virsh already goes to virtqemud directly
// on such hosts, by the rule above.
//
// An explicit socket (foundry --socket) replaces this choice. It is how
// foundry reaches a libvirt on another machine: the far end's socket forwarded
// to a local one over SSH
// (`ssh -L /tmp/libvirt.sock:/run/libvirt/virtqemud-sock user@host`), much as
// libvirt's own qemu+ssh transport does (https://libvirt.org/uri.html).
func DefaultSocket() string {
	return chooseSocket(fileExists)
}

// chooseSocket is DefaultSocket with the existence check injected.
func chooseSocket(exists func(path string) bool) string {
	if exists(QEMUSocket) {
		return QEMUSocket
	}
	return LegacySocket
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Connect establishes a connection to libvirt (qemu:///system).
// It returns a Client that must be closed via Close() when done.
//
// If socketPath is empty, the socket is Socket's choice: the one set with
// SetSocket, else virtqemud's when it exists, else the legacy libvirt-sock.
// If timeout is zero, defaults to 5 seconds.
func Connect(socketPath string, timeout time.Duration) (*Client, error) {
	if socketPath == "" {
		socketPath = Socket()
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

// ConnectWithContext establishes a connection with context support for
// cancellation. When ctx ends first, it returns at once; a connection that
// completes after that is closed, so a cancelled connect leaks nothing.
func ConnectWithContext(ctx context.Context, socketPath string, timeout time.Duration) (*Client, error) {
	// Choose the socket now, not in the goroutine, which may outlive this call
	if socketPath == "" {
		socketPath = Socket()
	}
	return connectWithContext(ctx, func() (*Client, error) { return Connect(socketPath, timeout) })
}

// connectWithContext is ConnectWithContext with the connect injected.
func connectWithContext(ctx context.Context, connect func() (*Client, error)) (*Client, error) {
	type result struct {
		client *Client
		err    error
	}
	resultCh := make(chan result, 1)

	go func() {
		c, err := connect()
		resultCh <- result{client: c, err: err}
	}()

	select {
	case <-ctx.Done():
		// The connect goroutine is still running: close what it makes
		go func() {
			if res := <-resultCh; res.err == nil && res.client != nil {
				_ = res.client.Close()
			}
		}()
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

// HostArch returns the architecture of the machine libvirt runs on, in
// libvirt's names (its capabilities' <host><cpu><arch>,
// https://libvirt.org/formatcaps.html). With a socket forwarded from another
// machine that is not the machine foundry runs on.
func (c *Client) HostArch() (string, error) {
	return HostArch(c.libvirt)
}

// CapabilitiesReader reads libvirt's host capabilities.
type CapabilitiesReader interface {
	ConnectGetCapabilities() (string, error)
}

// HostArch returns the architecture of the machine the libvirt behind lv
// runs on, as Client.HostArch.
func HostArch(lv CapabilitiesReader) (string, error) {
	xml, err := lv.ConnectGetCapabilities()
	if err != nil {
		return "", fmt.Errorf("failed to read host capabilities: %w", err)
	}
	var caps libvirtxml.Caps
	if err := caps.Unmarshal(xml); err != nil {
		return "", fmt.Errorf("failed to parse host capabilities: %w", err)
	}
	if caps.Host.CPU == nil || caps.Host.CPU.Arch == "" {
		return "", fmt.Errorf("host capabilities name no architecture")
	}
	return caps.Host.CPU.Arch, nil
}
