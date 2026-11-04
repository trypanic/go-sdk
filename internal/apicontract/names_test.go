package apicontract_test

import (
	"testing"

	"github.com/trypanic/go-sdk/v2/envs"
	"github.com/trypanic/go-sdk/v2/httprequest"
	"github.com/trypanic/go-sdk/v2/httpserver"
	"github.com/trypanic/go-sdk/v2/llmclient"
	"github.com/trypanic/go-sdk/v2/logger/otlp"
	"github.com/trypanic/go-sdk/v2/messaging"
	"github.com/trypanic/go-sdk/v2/postgres"
	"github.com/trypanic/go-sdk/v2/storage"
)

func TestConcisePublicNames(t *testing.T) {
	var _ httprequest.Requester = (*httprequest.Request)(nil)
	var _ httpserver.Server
	var _ httpserver.Context
	var _ llmclient.Provider = (*llmclient.Client)(nil)
	_ = llmclient.Request{}
	_ = llmclient.RequestConfig{}
	_ = llmclient.Response{}
	var _ storage.KVStore[string]
	var _ storage.AppendLog[string]
	var _ messaging.Broker = (*messaging.PubSub)(nil)
	var _ *otlp.Writer
	_ = postgres.New
	_ = postgres.NewContext
	_ = postgres.NewFromEnv
	_ = postgres.NewFromEnvContext
	var cfg struct{}
	if err := envs.LoadInto(&cfg); err != nil {
		t.Fatal(err)
	}
}
