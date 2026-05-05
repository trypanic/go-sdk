package storage

import "sync"

// MemoryStorage is a generic, thread-safe in-memory store. It implements
// both KVStorager (Put/Get with overwrite semantics) and AppendLogStorager
// (Append/List with append semantics) over disjoint internal maps.
type MemoryStorage[T any] struct {
	single map[string]T
	list   map[string][]T
	mu     sync.RWMutex
}

// NewMemory creates a new MemoryStorage.
func NewMemory[T any]() *MemoryStorage[T] {
	return &MemoryStorage[T]{
		single: make(map[string]T),
		list:   make(map[string][]T),
	}
}

// Put stores a single value at key, replacing any previous value.
func (ms *MemoryStorage[T]) Put(key string, value *T) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.single[key] = *value
	return nil
}

// Get retrieves the single value for key.
func (ms *MemoryStorage[T]) Get(key string) (*T, bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	value, ok := ms.single[key]
	if !ok {
		return nil, false, nil
	}
	return &value, true, nil
}

// Append appends an item to the list at key.
func (ms *MemoryStorage[T]) Append(key string, item *T) (*[]T, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.list[key] = append(ms.list[key], *item)
	result := ms.list[key]
	return &result, nil
}

// List retrieves the appended list for key.
func (ms *MemoryStorage[T]) List(key string) (*[]T, bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	slice, ok := ms.list[key]
	if !ok {
		return nil, false, nil
	}
	return &slice, true, nil
}

// Delete removes key from both single-value and list storage.
func (ms *MemoryStorage[T]) Delete(key string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.single, key)
	delete(ms.list, key)
	return nil
}

// Clear wipes both maps.
func (ms *MemoryStorage[T]) Clear() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.single = make(map[string]T)
	ms.list = make(map[string][]T)
	return nil
}
