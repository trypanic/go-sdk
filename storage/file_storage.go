package storage

import (
	"bytes"
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
// with a dot and do not end in a storage extension, so List, GetAll and
// Clear never treat them as storage files.
const tempFilePattern = ".tmp-*"

// Storage file extensions. Current files are JSON Lines (one JSON value per
// line, appended in place). Files written by earlier versions hold one JSON
// array; they are still read and are converted on the next Append.
const (
	fileExt       = ".jsonl"
	legacyFileExt = ".json"
)

// MaxFileKeyLength is the longest key, in bytes, that FileStorage accepts.
// Keys are hex-encoded (two characters per byte) and suffixed with ".jsonl",
// and common filesystems limit a file name to 255 bytes.
const MaxFileKeyLength = (255 - len(fileExt)) / 2

// FileStorage is an append-log file store. Each key maps to one JSON Lines
// file with one item per line. Append adds a line in place; List reads the
// lines back. Keys are hex-encoded so that arbitrary byte sequences map to
// distinct files.
//
// A FileStorage is safe for concurrent use. Use one FileStorage per directory
// per host: several instances on one directory never corrupt a file, but
// their writes are not coordinated, so one instance can lose an update made
// concurrently by another.
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

// Append adds item to the list at key and returns the full list after the
// append. The item is written as one line at the end of the key's file
// (O_APPEND, then fsync), so the cost of the write does not grow with the
// list. A new file, a file in the legacy array format and a file whose last
// line was torn by a crash are instead rewritten atomically. A nil item is
// rejected with an error, as in MemoryStorage.
func (s *FileStorage[T]) Append(key string, item *T) (*[]T, error) {
	if item == nil {
		return nil, nilInputError("Append: item must not be nil")
	}
	if err := validateFileKey(key); err != nil {
		return nil, err
	}
	line, err := json.Marshal(item)
	if err != nil {
		return nil, encodeError(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items, source, err := s.readKey(key)
	if err != nil {
		return nil, err
	}
	items = append(items, *item)

	if source == sourceLines {
		if err := appendLine(s.pathFor(key), line); err != nil {
			return nil, ioError("failed to append to storage file", err)
		}
		return &items, nil
	}
	if err := s.writeAll(key, items); err != nil {
		return nil, err
	}
	if source == sourceLegacy {
		// The new file shadows the legacy one, so a failed removal loses
		// nothing; Delete and Clear remove both.
		_ = os.Remove(s.legacyPathFor(key))
	}
	return &items, nil
}

// List returns the list stored at key. The bool is false (and error is nil)
// when the key is absent.
func (s *FileStorage[T]) List(key string) (*[]T, bool, error) {
	if err := validateFileKey(key); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	items, source, err := s.readKey(key)
	if err != nil {
		return nil, false, err
	}
	if source == sourceNone {
		return nil, false, nil
	}
	return &items, true, nil
}

// Delete removes the files at key. A missing key is not an error.
func (s *FileStorage[T]) Delete(key string) error {
	if err := validateFileKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, filename := range []string{s.pathFor(key), s.legacyPathFor(key)} {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ioError("failed to delete storage file", err)
		}
	}
	return nil
}

// Clear removes every storage file managed by this FileStorage: the files
// named after a hex-encoded key (the files GetAll returns), in the current
// or the legacy format. Other files in baseDir are left alone.
func (s *FileStorage[T]) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return ioError("failed to read storage directory", err)
	}
	for _, entry := range entries {
		if _, ok := ownedKey(entry); !ok {
			continue
		}
		if err := os.Remove(filepath.Join(s.baseDir, entry.Name())); err != nil {
			return ioError("failed to delete storage file", err)
		}
	}
	return nil
}

// GetAll returns a map of all stored lists, keyed by the original (decoded) key.
func (s *FileStorage[T]) GetAll() (map[string][]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, ioError("failed to read storage directory", err)
	}
	out := make(map[string][]T)
	for _, entry := range entries {
		key, ok := ownedKey(entry)
		if !ok {
			continue
		}
		if _, done := out[key]; done {
			continue // both formats exist; readKey already chose
		}
		items, source, err := s.readKey(key)
		if err != nil {
			return nil, err
		}
		if source != sourceNone {
			out[key] = items
		}
	}
	return out, nil
}

// pathFor returns the path of the key's current-format file.
func (s *FileStorage[T]) pathFor(key string) string {
	return filepath.Join(s.baseDir, fileNameForKey(key))
}

// legacyPathFor returns the path of the key's legacy array-format file.
func (s *FileStorage[T]) legacyPathFor(key string) string {
	return filepath.Join(s.baseDir, legacyFileNameForKey(key))
}

// fileNameForKey hex-encodes the raw key bytes so that every distinct key
// maps to a distinct filename, regardless of any characters it contains.
func fileNameForKey(key string) string {
	return hex.EncodeToString([]byte(key)) + fileExt
}

// legacyFileNameForKey is the file name earlier versions used for key.
func legacyFileNameForKey(key string) string {
	return hex.EncodeToString([]byte(key)) + legacyFileExt
}

// ownedKey reports whether a directory entry is a storage file of this
// FileStorage (a regular file named exactly as fileNameForKey or
// legacyFileNameForKey would name it) and returns its key. GetAll and Clear
// share this ownership rule.
func ownedKey(entry os.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	name := entry.Name()
	for _, ext := range []string{fileExt, legacyFileExt} {
		stem, found := strings.CutSuffix(name, ext)
		if !found {
			continue
		}
		raw, err := hex.DecodeString(stem)
		if err != nil || hex.EncodeToString(raw) != stem {
			return "", false
		}
		return string(raw), true
	}
	return "", false
}

// fileSource says where readKey found a key's items.
type fileSource int

const (
	sourceNone   fileSource = iota // no file for the key
	sourceLines                    // a complete JSON Lines file
	sourceTorn                     // a JSON Lines file whose last line was torn
	sourceLegacy                   // a legacy JSON array file
)

// readKey reads the key's items from its JSON Lines file or, when that file
// does not exist, from its legacy array file. A missing key yields an empty
// slice and sourceNone. Caller must hold the lock.
func (s *FileStorage[T]) readKey(key string) ([]T, fileSource, error) {
	data, err := os.ReadFile(s.pathFor(key))
	switch {
	case err == nil:
		return decodeLines[T](data)
	case !errors.Is(err, os.ErrNotExist):
		return nil, sourceNone, ioError("failed to read storage file", err)
	}

	data, err = os.ReadFile(s.legacyPathFor(key))
	switch {
	case err == nil:
		items, err := decodeLegacy[T](data)
		return items, sourceLegacy, err
	case errors.Is(err, os.ErrNotExist):
		return []T{}, sourceNone, nil
	default:
		return nil, sourceNone, ioError("failed to read storage file", err)
	}
}

// decodeLines decodes a JSON Lines file. Files are created atomically with
// at least one complete line, so an empty file or one without a single
// newline is corrupt (ERR_STORAGE_CORRUPTED), as is any complete line that
// does not decode. Bytes after the last newline are the remains of an
// append torn by a crash: that Append never returned, so they are ignored
// and reported as sourceTorn for the next Append to rewrite.
func decodeLines[T any](data []byte) ([]T, fileSource, error) {
	if len(data) == 0 {
		// Reading a zero-length file as an empty list would let the next
		// Append silently drop the key's history.
		return nil, sourceNone, corruptError("storage file is empty (corrupt)", nil)
	}
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return nil, sourceNone, corruptError("storage file has no complete record (corrupt)", nil)
	}
	source := sourceLines
	if end != len(data)-1 {
		source = sourceTorn
	}

	items := make([]T, 0, bytes.Count(data[:end+1], []byte{'\n'}))
	for line := range bytes.Lines(data[:end+1]) {
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, sourceNone, corruptError("failed to decode storage file", err)
		}
		items = append(items, item)
	}
	return items, source, nil
}

// decodeLegacy decodes a file in the legacy format: one JSON array.
func decodeLegacy[T any](data []byte) ([]T, error) {
	if len(data) == 0 {
		return nil, corruptError("storage file is empty (corrupt)", nil)
	}
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, corruptError("failed to decode storage file", err)
	}
	return items, nil
}

// writeAll atomically and durably replaces the key's file with items in the
// JSON Lines format: a fresh temp file created with O_EXCL (a unique,
// unpredictable name that never follows a symlink) is written, synced and
// renamed onto the final path, and the directory is synced so the rename
// survives a crash. Caller must hold the lock.
func (s *FileStorage[T]) writeAll(key string, items []T) error {
	var buf bytes.Buffer
	for i := range items {
		line, err := json.Marshal(&items[i])
		if err != nil {
			return encodeError(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := writeFileAtomic(s.baseDir, s.pathFor(key), buf.Bytes(), s.fileMode); err != nil {
		return ioError("failed to write storage file", err)
	}
	return nil
}

// appendLine writes line and a newline at the end of an existing file in a
// single write and syncs it.
func appendLine(filename string, line []byte) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func encodeError(err error) *errorkit.AppError {
	return errorkit.NewError(ERR_STORAGE_ERROR).With(
		errorkit.WithReason("failed to encode storage item"),
		errorkit.WithWrapped(err),
	)
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
