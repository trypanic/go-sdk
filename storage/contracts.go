package storage

// KVStore is a key-value store with overwrite semantics.
// Put replaces any previous value at the key.
type KVStore[T any] interface {
	Put(key string, value *T) error
	Get(key string) (*T, bool, error)
	Delete(key string) error
	Clear() error
}

// AppendLog is a key-list store with append semantics.
// Append adds an item to the list at key; List reads the full list.
type AppendLog[T any] interface {
	Append(key string, item *T) (*[]T, error)
	List(key string) (*[]T, bool, error)
	Delete(key string) error
	Clear() error
}
