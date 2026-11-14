package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"

	"github.com/digitalocean/go-libvirt"

	foundrylibvirt "github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/naming"
	"github.com/jbweber/foundry/internal/storage"
)

// LogSerial is the serial console log, the only console log foundry keeps.
const LogSerial = "serial"

// Logs writes a VM's serial console log to w, oldest first: the copies
// virtlogd rotated it into ({log}.N down to {log}.0), then the log itself.
// It reads the logs pool through libvirt (a pool refresh, then
// StorageVolDownload), so it works over a connection to a libvirt on another
// machine (--socket) as on the hypervisor itself. It does not need the VM's
// domain: a log is read while the VM runs, after it stopped, and when its
// domain is gone but the log is not (destroy deletes it). It creates no pool:
// without the logs pool there is no log to read.
//
// kind must be LogSerial (or empty, which means it).
func Logs(ctx context.Context, vmName, kind string, w io.Writer) error {
	if kind != "" && kind != LogSerial {
		return fmt.Errorf("unknown log %q: %s is the only console log", kind, LogSerial)
	}
	c, err := foundrylibvirt.ConnectWithContext(ctx, "", 0)
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("Warning: failed to close libvirt connection: %v", err)
		}
	}()
	return logsWithDeps(ctx, vmName, storage.NewManager(c.Libvirt()), w)
}

// logsWithDeps writes a VM's serial console log to w; see Logs.
func logsWithDeps(ctx context.Context, vmName string, sm storageManager, w io.Writer) error {
	parts, err := serialLogParts(ctx, sm, vmName)
	if err != nil {
		var le libvirt.Error
		if errors.As(err, &le) && le.Code == uint32(libvirt.ErrNoStoragePool) {
			return fmt.Errorf("no serial log for %s: there is no pool %s", vmName, storage.DefaultLogsPool)
		}
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("no serial log for %s in pool %s", vmName, storage.DefaultLogsPool)
	}
	for _, p := range parts {
		// virtlogd may rotate the log between the listing and the reads: a
		// copy that is gone by then is an error, and running logs again
		// reads the new set
		if err := sm.DownloadVolume(ctx, p.pool, p.name, w); err != nil {
			return err
		}
	}
	return nil
}

// serialLogParts refreshes the logs pool (virtlogd creates and rotates the
// files behind libvirt's back) and returns the VM's serial log files in the
// order they were written: the rotated copies from the highest number (the
// oldest) to {log}.0, then the log itself. The names are exact
// (naming.SerialLogCopy): no other VM's log matches.
func serialLogParts(ctx context.Context, sm storageManager, vmName string) ([]poolVolume, error) {
	if err := sm.RefreshPool(ctx, storage.DefaultLogsPool); err != nil {
		return nil, err
	}
	volumes, err := sm.ListVolumes(ctx, storage.DefaultLogsPool)
	if err != nil {
		return nil, err
	}
	type part struct {
		name string
		n    int
	}
	var parts []part
	for _, v := range volumes {
		if n, ok := naming.SerialLogCopy(v.Name, vmName); ok {
			parts = append(parts, part{v.Name, n})
		}
	}
	// Highest copy number first, the log itself (-1) last
	sort.Slice(parts, func(i, j int) bool { return parts[i].n > parts[j].n })
	found := make([]poolVolume, 0, len(parts))
	for _, p := range parts {
		found = append(found, poolVolume{pool: storage.DefaultLogsPool, name: p.name})
	}
	return found, nil
}

// serialLogVolumes returns the VM's serial log files in the logs pool for a
// destroy to delete; a pool that cannot be refreshed or listed (or does not
// exist) is skipped with a warning, as destroy skips other pools it cannot
// list.
func serialLogVolumes(ctx context.Context, sm storageManager, vmName string) []poolVolume {
	parts, err := serialLogParts(ctx, sm, vmName)
	if err != nil {
		log.Printf("Warning: failed to look for the serial log of %s in pool %s: %v", vmName, storage.DefaultLogsPool, err)
		return nil
	}
	return parts
}

// checkSerialLogAbsent fails if the VM's serial log, or a rotated copy of
// it, is already in the logs pool: left of an earlier VM of the same name
// (destroy could not delete it). The new VM would append to it, and its log
// would start with another VM's output, so create refuses the name, as it
// refuses a leftover volume; 'foundry destroy <vm>' removes what is left.
func checkSerialLogAbsent(ctx context.Context, vmName string, sm storageManager) error {
	parts, err := serialLogParts(ctx, sm, vmName)
	if err != nil {
		return fmt.Errorf("failed to check for an old serial log: %w", err)
	}
	if len(parts) > 0 {
		return fmt.Errorf("serial log already exists: %s/%s (left of an earlier VM of this name; 'foundry destroy %s' removes it)",
			parts[len(parts)-1].pool, parts[len(parts)-1].name, vmName)
	}
	return nil
}
