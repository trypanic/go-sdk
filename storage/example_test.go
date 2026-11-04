package storage_test

import (
	"fmt"
	"log"
	"os"

	"github.com/trypanic/go-sdk/storage"
)

func ExampleFileStorage() {
	dir, err := os.MkdirTemp("", "storage-example")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	store, err := storage.NewFileStorage[string](dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, event := range []string{"created", "paid"} {
		if _, err := store.Append("order-1", &event); err != nil {
			log.Fatal(err)
		}
	}

	events, ok, err := store.List("order-1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ok, *events)
	// Output: true [created paid]
}
