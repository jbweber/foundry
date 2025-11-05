package storage

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"
	"sync"
)

var (
	// Cached QEMU user/group IDs
	qemuUID  string
	qemuGID  string
	qemuOnce sync.Once
	qemuErr  error
)

// GetQEMUUserGroup returns the UID and GID for the QEMU process user.
// It attempts multiple strategies to determine the correct user:
// 1. Read from /etc/libvirt/qemu.conf to get the configured user/group
// 2. Fall back to common user names (qemu, libvirt-qemu)
// 3. Fall back to hardcoded values (107) as a last resort
//
// The result is cached after the first call.
func GetQEMUUserGroup() (uid, gid string, err error) {
	qemuOnce.Do(func() {
		// Try to get user/group from qemu.conf
		username, groupname := getQEMUConfiguredUser()

		// Try to look up the configured user
		if username != "" {
			u, err := user.Lookup(username)
			if err == nil {
				qemuUID = u.Uid
				// If a group was configured, look it up
				if groupname != "" {
					g, err := user.LookupGroup(groupname)
					if err == nil {
						qemuGID = g.Gid
					} else {
						// Fall back to user's primary GID
						qemuGID = u.Gid
					}
				} else {
					qemuGID = u.Gid
				}
				return
			}
		}

		// Fall back to trying common usernames
		commonUsers := []string{"qemu", "libvirt-qemu"}
		for _, username := range commonUsers {
			u, err := user.Lookup(username)
			if err == nil {
				qemuUID = u.Uid
				qemuGID = u.Gid
				return
			}
		}

		// Last resort: use hardcoded values (Fedora/RHEL default)
		qemuUID = "107"
		qemuGID = "107"
		qemuErr = fmt.Errorf("could not determine QEMU user/group, using fallback UID/GID 107")
	})

	return qemuUID, qemuGID, qemuErr
}

// getQEMUConfiguredUser reads /etc/libvirt/qemu.conf and extracts
// the configured user and group names.
// Returns empty strings if the file doesn't exist or the settings aren't found.
func getQEMUConfiguredUser() (username, groupname string) {
	file, err := os.Open("/etc/libvirt/qemu.conf")
	if err != nil {
		return "", ""
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log error but continue since we're returning data already read
			_ = closeErr
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Look for user = "username" or user = 'username'
		if strings.HasPrefix(line, "user") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				// Remove quotes
				value = strings.Trim(value, "\"'")
				username = value
			}
		}

		// Look for group = "groupname" or group = 'groupname'
		if strings.HasPrefix(line, "group") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				// Remove quotes
				value = strings.Trim(value, "\"'")
				groupname = value
			}
		}
	}

	return username, groupname
}
