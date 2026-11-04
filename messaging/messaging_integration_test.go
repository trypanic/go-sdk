//go:build integration

package messaging_test

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/internal/integrationtest"
	"github.com/trypanic/go-sdk/messaging"
)

func TestRabbitMQIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("requires local RabbitMQ container")
	}
	container := integrationtest.Start(t, testcontainers.ContainerRequest{
		Image: "rabbitmq:4.4.0-alpine@sha256:21577d148f4f7563a87d761862fa353f59348a094833ca8b1b1a46a8102dffce", ExposedPorts: []string{"5672/tcp"},
		Env:        map[string]string{"RABBITMQ_DEFAULT_USER": "sdk", "RABBITMQ_DEFAULT_PASS": "local-test-only"},
		WaitingFor: wait.ForLog("Server startup complete").WithStartupTimeout(90 * time.Second),
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5672/tcp")
	require.NoError(t, err)
	endpoint := (&url.URL{Scheme: "amqp", User: url.UserPassword("sdk", "local-test-only"), Host: net.JoinHostPort(host, port.Port()), Path: "/"}).String()
	topology := &messaging.Topology{
		Exchanges: []messaging.Exchange{{Name: "sdk.events", Type: "direct"}},
		Queues:    []messaging.QueueTopology{{Name: "sdk.received", Durable: true, Prefetch: 1}},
		Bindings:  []messaging.Binding{{Queue: "sdk.received", Exchange: "sdk.events", RoutingKeys: []string{"created"}}},
	}
	setup, finishSetup := context.WithCancel(ctx)
	defer finishSetup()
	bus, err := messaging.NewPubSubContext(setup, endpoint, messaging.WithTopology(topology), messaging.WithDeclareTopology(), messaging.WithInstrumenter(nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bus.Close()) })
	finishSetup()
	t.Run("confirmed publish and consume survive setup cancellation", func(t *testing.T) {
		event := messaging.Event{CorrelationID: "integration", ServiceName: "sdk-test", Payload: map[string]any{"name": "first"}}
		require.NoError(t, bus.Publish(ctx, "sdk.received", event))
		received := make(chan messaging.Message, 1)
		done := make(chan error, 1)
		subscription, stop := context.WithCancel(ctx)
		defer stop()
		go func() {
			done <- bus.Subscribe(subscription, "sdk.received", "integration", func(_ context.Context, msg messaging.Message) error {
				select {
				case received <- msg:
				case <-subscription.Done():
				}
				return nil
			})
		}()
		t.Cleanup(func() {
			stop()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Error("subscription failed to stop")
			}
		})
		select {
		case message := <-received:
			var got messaging.EventMessage
			require.NoError(t, json.Unmarshal(message.Raw, &got))
			require.NotEmpty(t, got.EventID)
			require.Equal(t, event.Payload, got.Payload)
			require.Equal(t, event.CorrelationID, got.CorrelationID)
			require.Equal(t, "created", message.RoutingKey)
		case <-ctx.Done():
			t.Fatal("confirmed message was not delivered")
		}
	})
	t.Run("mandatory returns are failures", func(t *testing.T) {
		unbound := &messaging.Topology{Exchanges: []messaging.Exchange{{Name: "sdk.unbound", Type: "direct"}}, Queues: []messaging.QueueTopology{{Name: "sdk.missing", Retry: &messaging.RetryConfig{MaxAttempts: 1, BackoffMult: 1}}}, Bindings: []messaging.Binding{{Queue: "sdk.missing", Exchange: "sdk.unbound", RoutingKeys: []string{"missing"}}}}
		publisher, err := messaging.NewPubSubContext(ctx, endpoint, messaging.WithTopology(unbound))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, publisher.Close()) })
		err = publisher.Publish(ctx, "sdk.missing", messaging.Event{})
		require.True(t, errorkit.HasCode(err, messaging.ERR_MQ_PUBLISH_FAILED), "%v", err)
	})
	t.Run("close stops an active subscription and is terminal", func(t *testing.T) {
		entered := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- bus.Subscribe(ctx, "sdk.received", "closing", func(handlerCtx context.Context, _ messaging.Message) error {
				close(entered)
				<-handlerCtx.Done()
				return handlerCtx.Err()
			})
		}()
		require.NoError(t, bus.Publish(ctx, "sdk.received", messaging.Event{}))
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("handler did not start")
		}
		require.NoError(t, bus.Close())
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("Close did not stop subscription")
		}
		require.NoError(t, bus.Close())
		require.True(t, errorkit.HasCode(bus.Publish(ctx, "sdk.received", messaging.Event{}), messaging.ERR_MQ_CLOSED))
		require.True(t, errorkit.HasCode(bus.Subscribe(ctx, "sdk.received", "closed", func(context.Context, messaging.Message) error { return nil }), messaging.ERR_MQ_CLOSED))
	})
}
