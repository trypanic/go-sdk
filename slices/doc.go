// Package slices provides slice helpers not covered by the standard library:
// Chunk splits a slice into batches and MergeUniqueStrings merges two string
// slices without duplicates.
//
// The package name is the same as the standard library package slices; a
// file that needs both must alias one of them, for example
// sdkslices "github.com/trypanic/go-sdk/slices".
package slices
