//go:build !unix

package storage

// openSafeFlags is empty where O_NOFOLLOW does not exist; openRegular then
// relies on the Lstat and os.SameFile checks alone.
const openSafeFlags = 0

func isSymlinkOpenError(error) bool { return false }
