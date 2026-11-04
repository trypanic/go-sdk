package httprequest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/telemetry"
)

var _ func(*http.Client) *httprequest.Request = httprequest.New

func TestConcreteRequesterAndUnwrap(t *testing.T) {
	requester := httprequest.New(&http.Client{})
	wrapped := httprequest.WrapWithInstrumenter(requester, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
	unwrappable, ok := wrapped.(interface {
		Unwrap() httprequest.Requester
	})
	require.True(t, ok)
	require.Same(t, requester, unwrappable.Unwrap())
}
