package messagingtest_test

import (
	"context"
	"fmt"

	"github.com/trypanic/go-sdk/messaging"
	"github.com/trypanic/go-sdk/messaging/messagingtest"
)

func ExampleBroker() {
	broker := &messagingtest.Broker{PublishFunc: func(_ context.Context, queue string, _ messaging.Event) error { fmt.Println(queue); return nil }}
	if err := broker.Publish(context.Background(), "jobs", messaging.Event{}); err != nil {
		fmt.Println(err)
	}
	// Output: jobs
}
