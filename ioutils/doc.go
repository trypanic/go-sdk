// Package ioutils writes JSON for local development and debugging.
//
// Do not use this package on production data paths. Prefer storage for
// persistence and marshal for wire encoding. WriteJSON accepts a caller-owned
// writer; SaveJSON creates private files and returns classified failures.
package ioutils
