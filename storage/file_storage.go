package storage

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/trypanic/go-sdk/errorkit"
)

// FileStorage is an append-log file store. Each key maps to a single JSON
// file containing an []T value. Append adds to that list; List reads it.
// Keys are hex-encoded so that arbitrary byte sequences map to distinct files.
type FileStorage[T any] struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStorage creates a FileStorage rooted at baseDir, creating it if
// missing.
func NewFileStorage[T any](baseDir string) (*FileStorage[T], error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to create storage directory"),
			errorkit.WithWrapped(err),
		)
	}
	return &FileStorage[T]{baseDir: baseDir}, nil
}

// Append adds item to the list at key, creating the file if necessary.
// Returns the full list after the append.
func (fs *FileStorage[T]) Append(key string, item *T) (*[]T, error) {
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
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filename := fs.pathFor(key)
	if _, err := os.Stat(filename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to stat storage file"),
			errorkit.WithWrapped(err),
		)
	}
	slice, err := fs.readUnsafe(filename)
	if err != nil {
		return nil, false, err
	}
	return &slice, true, nil
}

// Delete removes the file at key. A missing key is not an error.
func (fs *FileStorage[T]) Delete(key string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	filename := fs.pathFor(key)
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to delete storage file"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// Clear removes every storage file managed by this FileStorage.
func (fs *FileStorage[T]) Clear() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to read storage directory"),
			errorkit.WithWrapped(err),
		)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(fs.baseDir, name)); err != nil {
			return errorkit.NewError(ERR_STORAGE_ERROR).With(
				errorkit.WithReason("failed to delete storage file"),
				errorkit.WithWrapped(err),
			)
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
		return nil, errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to read storage directory"),
			errorkit.WithWrapped(err),
		)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		key, ok := keyFromFileName(name)
		if !ok {
			continue
		}
		slice, err := fs.readUnsafe(filepath.Join(fs.baseDir, name))
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
// empty slice; a read or parse failure surfaces as ERR_STORAGE_ERROR.
// Caller must hold the lock.
func (fs *FileStorage[T]) readUnsafe(filename string) ([]T, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []T{}, nil
		}
		return nil, errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to read storage file"),
			errorkit.WithWrapped(err),
		)
	}
	if len(data) == 0 {
		return []T{}, nil
	}
	var result []T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to decode storage file"),
			errorkit.WithWrapped(err),
		)
	}
	return result, nil
}

// writeUnsafe atomically writes the slice to the file.
// Caller must hold the lock.
func (fs *FileStorage[T]) writeUnsafe(filename string, slice []T) error {
	jsonData, err := json.MarshalIndent(slice, "", "  ")
	if err != nil {
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to encode storage file"),
			errorkit.WithWrapped(err),
		)
	}
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0o644); err != nil {
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to write storage file"),
			errorkit.WithWrapped(err),
		)
	}
	if err := os.Rename(tempFile, filename); err != nil {
		_ = os.Remove(tempFile)
		return errorkit.NewError(ERR_STORAGE_ERROR).With(
			errorkit.WithReason("failed to commit storage file"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}
