package telemetry_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/llmclient"
	"github.com/trypanic/go-sdk/mongo"
	"github.com/trypanic/go-sdk/mongo/mongotest"
	"github.com/trypanic/go-sdk/postgres"
	"github.com/trypanic/go-sdk/telemetry"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type failingRequest struct{ err error }

func (f failingRequest) Do(context.Context, httprequest.RequestConfig, any) error { return f.err }

type failingLLM struct{ err error }

func (f failingLLM) Execute(context.Context, llmclient.RequestConfig) (string, error) {
	return "", f.err
}

type failingProcedure struct{ err error }

func (f failingProcedure) QueryRow(context.Context, string, ...any) (int, error) { return 0, f.err }
func (f failingProcedure) Query(context.Context, string, ...any) ([]int, error)  { return nil, f.err }
func (f failingProcedure) QueryRowJSON(context.Context, string, any, ...any) (int, error) {
	return 0, f.err
}
func (f failingProcedure) QueryJSON(context.Context, string, any, ...any) ([]int, error) {
	return nil, f.err
}

func failingMongo(err error) *mongotest.Client {
	return &mongotest.Client{
		PingFunc:  func(context.Context) error { return err },
		CloseFunc: func(context.Context) error { return err },
		CollectionFunc: func(string) mongo.Collection {
			return &mongotest.Collection{
				CountDocumentsFunc: func(context.Context, any, ...options.Lister[options.CountOptions]) (int64, error) { return 0, err },
				FindOneFunc: func(context.Context, any, ...options.Lister[options.FindOneOptions]) mongo.SingleResult {
					return mongotest.SingleResult(func(any) error { return err })
				},
			}
		},
	}
}

func TestAdaptersRecordFailures(t *testing.T) {
	failure := errorkit.NewError(errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE)
	ctx := t.Context()
	cases := map[string]func(*telemetry.Instrumenter) error{
		"HTTP wrapper": func(i *telemetry.Instrumenter) error {
			return httprequest.WrapWithInstrumenter(failingRequest{failure}, i).Do(ctx, httprequest.RequestConfig{}, nil)
		},
		"HTTP concrete": func(i *telemetry.Instrumenter) error {
			return httprequest.NewWithInstrumenter(&http.Client{}, i).Do(ctx, httprequest.RequestConfig{URL: ":invalid"}, nil)
		},
		"LLM wrapper": func(i *telemetry.Instrumenter) error {
			_, err := llmclient.WrapWithInstrumenter(failingLLM{failure}, i).Execute(ctx, llmclient.RequestConfig{})
			return err
		},
		"LLM concrete": func(i *telemetry.Instrumenter) error {
			_, err := llmclient.NewWithInstrumenter(failingRequest{failure}, llmclient.Config{APIKey: "test", Endpoint: "https://example.invalid"}, i).Execute(ctx, llmclient.RequestConfig{Model: "model"})
			return err
		},
		"Mongo ping": func(i *telemetry.Instrumenter) error {
			return mongo.WrapWithInstrumenter(failingMongo(failure), i).Ping(ctx)
		},
		"Mongo close": func(i *telemetry.Instrumenter) error {
			return mongo.WrapWithInstrumenter(failingMongo(failure), i).Close(ctx)
		},
		"Mongo count": func(i *telemetry.Instrumenter) error {
			_, err := mongo.WrapWithInstrumenter(failingMongo(failure), i).Collection("items").CountDocuments(ctx, nil)
			return err
		},
		"Mongo findOne decode": func(i *telemetry.Instrumenter) error {
			return mongo.WrapWithInstrumenter(failingMongo(failure), i).Collection("items").FindOne(ctx, nil).Decode(nil)
		},
		"Postgres row": func(i *telemetry.Instrumenter) error {
			_, err := postgres.WrapWithInstrumenter[int](failingProcedure{failure}, i).QueryRow(ctx, "SELECT * FROM proc()")
			return err
		},
		"Postgres rows": func(i *telemetry.Instrumenter) error {
			_, err := postgres.WrapWithInstrumenter[int](failingProcedure{failure}, i).Query(ctx, "SELECT * FROM proc()")
			return err
		},
		"Postgres JSON row": func(i *telemetry.Instrumenter) error {
			_, err := postgres.WrapWithInstrumenter[int](failingProcedure{failure}, i).QueryRowJSON(ctx, "SELECT * FROM proc()", nil)
			return err
		},
		"Postgres JSON rows": func(i *telemetry.Instrumenter) error {
			_, err := postgres.WrapWithInstrumenter[int](failingProcedure{failure}, i).QueryJSON(ctx, "SELECT * FROM proc()", nil)
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			require.Error(t, run(telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})))
			spans := recorder.Ended()
			require.NotEmpty(t, spans)
			span := spans[len(spans)-1]
			require.Equal(t, codes.Error, span.Status().Code)
			require.Equal(t, trace.SpanKindClient, span.SpanKind())
			require.Len(t, span.Events(), 1)
			if name == "Mongo findOne decode" {
				require.Len(t, spans, 2)
				require.Equal(t, spans[0].SpanContext().SpanID(), span.Parent().SpanID())
			}
		})
	}
}
