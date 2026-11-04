// Package storage provides generic key-based stores: MemoryStorage (in
// memory, with overwrite and append semantics) and FileStorage (an append log
// persisted as one JSON Lines file per key).
//
// KVStorager means overwrite (Put/Get); AppendLogStorager means append
// (Append/List). Every failure is an *errorkit.AppError.
//
// Both stores are safe for concurrent use. Construct them with NewMemory and
// NewFileStorage; use one FileStorage per directory per host.
package storage
