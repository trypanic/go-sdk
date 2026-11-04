package mongo_test

import (
	"context"
	"log"

	"github.com/trypanic/go-sdk/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type product struct {
	Name  string `bson:"name"`
	Price int    `bson:"price"`
}

// ExampleCursor streams a large result instead of buffering it with All. It
// needs a server, so it is compiled but not run.
func ExampleCursor() {
	ctx := context.Background()
	client, err := mongo.New(ctx, mongo.Config{URI: "mongodb://localhost:27017", Database: "shop"})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close(ctx) }()

	cursor, err := client.Collection("products").Find(ctx, bson.D{})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var p product
		if err := cursor.Decode(&p); err != nil {
			log.Fatal(err) // ERR_DB_MONGO_DECODE_FAILED
		}
		log.Printf("%s: %d", p.Name, p.Price)
	}
	if err := cursor.Err(); err != nil {
		log.Fatal(err)
	}
}
