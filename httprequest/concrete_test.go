package httprequest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/telemetry"
)

func TestConcreteRequesterAndUnwrap(t *testing.T) {
	var requester *httprequest.Request = httprequest.New(&http.Client{})
	wrapped := httprequest.WrapWithInstrumenter(requester, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
	unwrappable, ok := wrapped.(interface {
		Unwrap() httprequest.Requester
	})
	require.True(t, ok)
	require.Same(t, requester, unwrappable.Unwrap())
}
