package storage

import (
	"fmt"
	"testing"
)

type benchRecord struct {
	ID      int    `json:"id"`
	Payload string `json:"payload"`
}

// BenchmarkFileStorageAppend measures building an append log of n records
// on one key (C4-017). With a full-file rewrite per Append the cost per log
// grows quadratically with n.
func BenchmarkFileStorageAppend(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("records=%d", n), func(b *testing.B) {
			record := benchRecord{Payload: "0123456789abcdef0123456789abcdef0123456789abcdef"}
			for b.Loop() {
				b.StopTimer()
				store, err := NewFileStorage[benchRecord](b.TempDir())
				if err != nil {
					b.Fatalf("NewFileStorage: %v", err)
				}
				b.StartTimer()
				for i := range n {
					record.ID = i
					if _, err := store.Append("log", &record); err != nil {
						b.Fatalf("Append: %v", err)
					}
				}
			}
		})
	}
}
