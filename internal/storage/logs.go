package storage

import (
	"context"
	"fmt"
	"io"
)

// LogsDir returns the directory of the logs pool (DefaultLogsPool) as libvirt
// reports it: the directory a new VM's serial console log is written to. The
// path is not fixed in foundry: a logs pool defined elsewhere before foundry
// first ran is used where it is, and the domain XML names the file in it, so
// the log is always a volume of the pool that `foundry logs` reads and
// `foundry destroy` deletes from. The pool is checked as CheckDirPool checks
// any pool foundry writes to.
func (m *Manager) LogsDir(ctx context.Context) (string, error) {
	return m.CheckDirPool(ctx, DefaultLogsPool)
}

// DownloadVolume writes a volume's bytes, as stored, to w: from its start to
// its end when it is read, so a file that grows while it is read (a console
// log virtlogd appends to) is read as far as it went then. It reads through
// libvirt (StorageVolDownload), so it works over a connection to a libvirt
// on another machine, and foundry opens no file on the hypervisor itself.
// The download is streamed. When ctx ends, it stops with ctx's error.
func (m *Manager) DownloadVolume(ctx context.Context, poolName, volumeName string, w io.Writer) error {
	pool, err := m.client.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}
	vol, err := m.client.StorageVolLookupByName(pool, volumeName)
	if err != nil {
		return fmt.Errorf("volume not found: %w", err)
	}
	// Length 0 reads to the end of the volume
	if err := m.client.StorageVolDownload(vol, ctxWriter{ctx, w}, 0, 0, 0); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("reading volume %s/%s stopped: %w", poolName, volumeName, ctxErr)
		}
		return fmt.Errorf("failed to read volume %s/%s: %w", poolName, volumeName, err)
	}
	return nil
}

// ctxWriter writes to w until ctx ends.
type ctxWriter struct {
	ctx context.Context
	w   io.Writer
}

func (c ctxWriter) Write(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.w.Write(p)
}
