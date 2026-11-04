package storage

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/trypanic/go-sdk/errorkit"
)

// tempFilePattern names the temp files used for atomic writes. They start
// with a dot and do not end in ".json", so List, GetAll and Clear never treat
// them as storage files.
const tempFilePattern = ".tmp-*"

// MaxFileKeyLength is the longest key, in bytes, that FileStorage accepts.
// Keys are hex-encoded (two characters per byte) and suffixed with ".json",
// and common filesystems limit a file name to 255 bytes.
const MaxFileKeyLength = (255 - len(".json")) / 2

// FileStorage is an append-log file store. Each key maps to a single JSON
// file containing an []T value. Append adds to that list; List reads it.
// Keys are hex-encoded so that arbitrary byte sequences map to distinct files.
//
// A FileStorage is safe for concurrent use. Use one FileStorage per directory
// per host: writes are atomic, so several instances on one directory never
// corrupt a file, but their read-modify-write cycles are not coordinated.
type FileStorage[T any] struct {
	baseDir  string
	fileMode fs.FileMode
	mu       sync.RWMutex
}

// Default permissions: stored items may hold personal data or tokens, so
// only the owning account can read them.
const (
	DefaultDirMode  fs.FileMode = 0o700
	DefaultFileMode fs.FileMode = 0o600
)

// FileStorageOption configures NewFileStorage.
type FileStorageOption func(*fileStorageConfig)

type fileStorageConfig struct {
	dirMode  fs.FileMode
	fileMode fs.FileMode
}

// WithDirMode sets the permissions of baseDir when NewFileStorage creates
// it (default DefaultDirMode). The process umask applies, and an existing
// directory keeps its permissions.
func WithDirMode(mode fs.FileMode) FileStorageOption {
	return func(c *fileStorageConfig) { c.dirMode = mode.Perm() }
}

// WithFileMode sets the permissions of the storage files (default
// DefaultFileMode). They are applied exactly, regardless of the umask.
func WithFileMode(mode fs.FileMode) FileStorageOption {
	return func(c *fileStorageConfig) { c.fileMode = mode.Perm() }
}

// NewFileStorage creates a FileStorage rooted at baseDir, creating it if
// missing. Directories are created DefaultDirMode and files written
// DefaultFileMode unless WithDirMode or WithFileMode says otherwise.
func NewFileStorage[T any](baseDir string, opts ...FileStorageOption) (*FileStorage[T], error) {
	cfg := fileStorageConfig{dirMode: DefaultDirMode, fileMode: DefaultFileMode}
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := os.MkdirAll(baseDir, cfg.dirMode); err != nil {
		return nil, ioError("failed to create storage directory", err)
	}
	return &FileStorage[T]{baseDir: baseDir, fileMode: cfg.fileMode}, nil
}

// Append adds item to the list at key, creating the file if necessary.
// Returns the full list after the append. A nil item is rejected with an
// error, as in MemoryStorage.
func (fs *FileStorage[T]) Append(key string, item *T) (*[]T, error) {
	if item == nil {
		return nil, nilInputError("Append: item must not be nil")
	}
	if err := validateFileKey(key); err != nil {
		return nil, err
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	filename := fs.pathFor(key)
	existing, err := fs.readUnsafe(filename)
	if err != nil {
		return nil, err
	}
	existing = append(existing, *item)
	if err := fs.writeUnsafe(filename, existing); err != nil {
		return nil, err
	}
	return &existing, nil
}

// List returns the list stored at key. The bool is false (and error is nil)
// when the key is absent.
func (fs *FileStorage[T]) List(key string) (*[]T, bool, error) {
	if err := validateFileKey(key); err != nil {
		return nil, false, err
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filename := fs.pathFor(key)
	if _, err := os.Stat(filename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, ioError("failed to stat storage file", err)
	}
	slice, err := fs.readUnsafe(filename)
	if err != nil {
		return nil, false, err
	}
	return &slice, true, nil
}

// Delete removes the file at key. A missing key is not an error.
func (fs *FileStorage[T]) Delete(key string) error {
	if err := validateFileKey(key); err != nil {
		return err
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	filename := fs.pathFor(key)
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ioError("failed to delete storage file", err)
	}
	return nil
}

// Clear removes every storage file managed by this FileStorage: the files
// whose names are hex-encoded keys (the files GetAll returns). Other files
// in baseDir are left alone.
func (fs *FileStorage[T]) Clear() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return ioError("failed to read storage directory", err)
	}
	for _, entry := range entries {
		if _, ok := ownedKey(entry); !ok {
			continue
		}
		if err := os.Remove(filepath.Join(fs.baseDir, entry.Name())); err != nil {
			return ioError("failed to delete storage file", err)
		}
	}
	return nil
}

// GetAll returns a map of all stored lists, keyed by the original (decoded) key.
func (fs *FileStorage[T]) GetAll() (map[string][]T, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	out := make(map[string][]T)
	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return nil, ioError("failed to read storage directory", err)
	}
	for _, entry := range entries {
		key, ok := ownedKey(entry)
		if !ok {
			continue
		}
		slice, err := fs.readUnsafe(filepath.Join(fs.baseDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		out[key] = slice
	}
	return out, nil
}

// pathFor returns the absolute file path for a key.
func (fs *FileStorage[T]) pathFor(key string) string {
	return filepath.Join(fs.baseDir, fileNameForKey(key))
}

// fileNameForKey hex-encodes the raw key bytes so that every distinct key
// maps to a distinct filename, regardless of any characters it contains.
func fileNameForKey(key string) string {
	return hex.EncodeToString([]byte(key)) + ".json"
}

// ownedKey reports whether a directory entry is a storage file of this
// FileStorage (a regular file named by fileNameForKey) and returns its key.
// GetAll and Clear share this ownership rule.
func ownedKey(entry os.DirEntry) (string, bool) {
	name := entry.Name()
	if entry.IsDir() || !strings.HasSuffix(name, ".json") {
		return "", false
	}
	return keyFromFileName(name)
}

// keyFromFileName decodes a filename produced by fileNameForKey back to the
// original key. Returns false for files whose names are not valid encodings.
func keyFromFileName(name string) (string, bool) {
	stem := strings.TrimSuffix(name, ".json")
	raw, err := hex.DecodeString(stem)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// readUnsafe reads and unmarshals the JSON file. A missing file yields an
// empty slice; a read or parse failure, or a zero-length file, surfaces as
// ERR_STORAGE_ERROR.
// Caller must hold the lock.
func (fs *FileStorage[T]) readUnsafe(filename string) ([]T, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []T{}, nil
		}
		return nil, ioError("failed to read storage file", err)
	}
	if len(data) == 0 {
		// Writes are atomic renames of non-empty files, so a zero-length
		// file means the data was lost (for example by a crash before the
		// rename reached the disk). Reading it as an empty list would let
		// the next Append silently drop the key's history.
		return nil, corruptError("storage file is empty (corrupt)", nil)
	}
	var result []T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, corruptError("failed to decode storage file", err)
	}
	return result, nil
}

// writeUnsafe atomically and durably replaces the file with the encoded
// slice. The data goes to a fresh temp file created with O_EXCL (a unique,
// unpredictable name that never follows a symlink), is synced, and is then
// renamed onto the final path; the directory is synced afterwards so the
// rename survives a crash. Concurrent writers never share a temp file.
// Caller must hold the lock.
func (fs *FileStorage[T]) writeUnsafe(filename string, slice []T) error {
	jsonData, err := json.MarshalIndent(slice, "", "  ")
	if err != nil {
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to encode storage file"),
			errorkit.WithWrapped(err),
		)
	}
	if err := writeFileAtomic(fs.baseDir, filename, jsonData, fs.fileMode); err != nil {
		return ioError("failed to write storage file", err)
	}
	return nil
}

// writeFileAtomic writes data to a unique temp file in dir and renames it
// onto filename with the given permissions. The temp file is removed on
// every failure path.
func writeFileAtomic(dir, filename string, data []byte, mode fs.FileMode) (err error) {
	tmp, err := os.CreateTemp(dir, tempFilePattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	// CreateTemp always uses 0o600; apply the configured mode exactly.
	if mode != 0o600 {
		if err = tmp.Chmod(mode); err != nil {
			_ = tmp.Close()
			return err
		}
	}

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, filename); err != nil {
		return err
	}
	return syncDir(dir)
}

// syncDir flushes a directory entry change (create, rename) to disk.
// Windows cannot sync a directory handle, so it is a no-op there.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
