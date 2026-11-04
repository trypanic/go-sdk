package hertz

import (
	"context"
	"errors"
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

func TestReplyWithErrorHidesInternalTextAndLogsIt(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, testOptions())
	s.GET("/fail", func(ctx context.Context, c httpserver.HTTPContext) {
		c.Reply(http.StatusInternalServerError, c.WithError(errors.New("db password=hunter2 host=db.internal")))
	})
	ctx, logs := logContext(t)

	res := perform(ctx, s, http.MethodGet, "/fail", nil)

	require.Equal(t, http.StatusInternalServerError, res.StatusCode())
	require.NotContains(t, string(res.Body()), "hunter2")
	require.Contains(t, string(res.Body()), "Internal server error")
	require.Contains(t, logs.String(), "hunter2", "the full error is logged server-side")
}

func TestReplyWithClientAppError(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, testOptions())
	s.GET("/bad", func(ctx context.Context, c httpserver.HTTPContext) {
		c.Reply(http.StatusBadRequest, c.WithError(errorkit.NewError(errorkit.ERR_CLIENT_BAD_REQUEST).
			With(errorkit.WithReason("email is required"))))
	})
	ctx, _ := logContext(t)

	res := perform(ctx, s, http.MethodGet, "/bad", nil)

	require.Equal(t, http.StatusBadRequest, res.StatusCode())
	require.JSONEq(t, `{"code":"ERR_CLIENT_BAD_REQUEST","error":"email is required"}`, stripTimestamp(t, res.Body()))
}

func TestPrettyJSONMarshalFailureHidesErrorText(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, testOptions())
	s.GET("/pretty", func(ctx context.Context, c httpserver.HTTPContext) {
		c.(*hertzContext).PrettyJSON(http.StatusOK, map[string]float64{"x": math.Inf(1)})
	})
	ctx, logs := logContext(t)

	res := perform(ctx, s, http.MethodGet, "/pretty", nil)

	require.Equal(t, http.StatusInternalServerError, res.StatusCode())
	require.NotContains(t, string(res.Body()), "unsupported value")
	require.Contains(t, logs.String(), "unsupported value")
}
