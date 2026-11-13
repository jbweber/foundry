package libvirt

import (
	"context"
	"errors"
	"testing"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/libvirttest"
)

// TestConnectWithContext_LateConnectClosed: a connect that completes after
// its context ended is closed, not leaked.
func TestConnectWithContext_LateConnectClosed(t *testing.T) {
	l := golibvirt.NewWithDialer(libvirttest.New())
	release := make(chan struct{})
	connected := make(chan struct{})
	connect := func() (*Client, error) {
		<-release
		if err := l.Connect(); err != nil {
			return nil, err
		}
		close(connected)
		return &Client{libvirt: l}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, err := connectWithContext(ctx, connect)
	if c != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, %v; want nil and context.Canceled", c, err)
	}

	close(release)
	<-connected
	deadline := time.Now().Add(5 * time.Second)
	for l.IsConnected() {
		if time.Now().After(deadline) {
			t.Fatal("the late connection was not closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestConnectWithContext_Connects: without a cancel, the connection is
// returned open.
func TestConnectWithContext_Connects(t *testing.T) {
	l := golibvirt.NewWithDialer(libvirttest.New())
	c, err := connectWithContext(context.Background(), func() (*Client, error) {
		return &Client{libvirt: l}, l.Connect()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if !c.Libvirt().IsConnected() {
		t.Error("connection not open")
	}
}
