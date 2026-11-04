//go:build unix

package storage

import (
	"errors"
	"syscall"
)

// openSafeFlags never follows a symlink at the final path component and
// never blocks on a FIFO swapped in after the Lstat check.
const openSafeFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// isSymlinkOpenError reports the error O_NOFOLLOW returns for a symlink
// (ELOOP on Linux and macOS, EMLINK on FreeBSD).
func isSymlinkOpenError(err error) bool {
	return errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK)
}
