package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

func TestWithErrorOptNeverSendsInternalErrorText(t *testing.T) {
	t.Parallel()

	internal := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
		With(errorkit.WithReason("panic recovered: secret=xyz"))
	clientErr := errorkit.NewError(errorkit.ERR_CLIENT_BAD_REQUEST).
		With(errorkit.WithReason("email is required"))
	wrappedClient := errorkit.NewError(errorkit.ERR_CLIENT_BAD_REQUEST).
		With(errorkit.WithWrapped(errors.New("json: cannot unmarshal at /srv/app/handler.go")))

	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantError string
		forbidden string
	}{
		{
			name:      "plain error",
			err:       errors.New(`pgx: SELECT * FROM users WHERE host=db.internal`),
			wantCode:  string(errorkit.ERR_INTERNAL),
			wantError: "Internal server error",
			forbidden: "db.internal",
		},
		{
			name:      "wrapped plain error",
			err:       fmt.Errorf("load user: %w", errors.New("dial tcp 10.0.0.7:5432")),
			wantCode:  string(errorkit.ERR_INTERNAL),
			wantError: "Internal server error",
			forbidden: "10.0.0.7",
		},
		{
			name:      "internal AppError uses the stable description",
			err:       internal,
			wantCode:  string(errorkit.ERR_SYSTEM_UNEXPECTED),
			wantError: internal.Metadata.Description,
			forbidden: "secret=xyz",
		},
		{
			name:      "AppError wrapped by fmt.Errorf",
			err:       fmt.Errorf("handler: %w", internal),
			wantCode:  string(errorkit.ERR_SYSTEM_UNEXPECTED),
			wantError: internal.Metadata.Description,
			forbidden: "secret=xyz",
		},
		{
			name:      "client AppError keeps its reason",
			err:       clientErr,
			wantCode:  string(errorkit.ERR_CLIENT_BAD_REQUEST),
			wantError: "email is required",
		},
		{
			name:      "client AppError without reason uses the description",
			err:       wrappedClient,
			wantCode:  string(errorkit.ERR_CLIENT_BAD_REQUEST),
			wantError: wrappedClient.Metadata.Description,
			forbidden: "/srv/app",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := BuildReply(ReplyOptions{}, WithErrorOpt(tt.err))
			body, err := json.Marshal(r)
			require.NoError(t, err)

			require.NotNil(t, r.Error)
			require.Equal(t, tt.wantError, *r.Error)
			require.Equal(t, tt.wantCode, r.Code)
			if tt.forbidden != "" {
				require.NotContains(t, string(body), tt.forbidden)
			}
		})
	}
}

func TestWithErrorOptCopiesTraceIDAndIgnoresNil(t *testing.T) {
	t.Parallel()

	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithTraceID("trace-1"))
	r := BuildReply(ReplyOptions{}, WithErrorOpt(appErr))
	require.Equal(t, "trace-1", r.TraceID)

	r = BuildReply(ReplyOptions{}, WithErrorOpt(nil))
	require.Nil(t, r.Error)
	require.Empty(t, r.Code)
}

func TestNilDataAndMetadataAreOmitted(t *testing.T) {
	t.Parallel()

	r := buildReply(ReplyOptions{}, WithDataOpt(nil), WithMetadataOpt(nil), WithMessageOpt("ok"))
	b, err := json.Marshal(r)
	require.NoError(t, err)
	require.JSONEq(t, `{"message":"ok"}`, string(b), "omitempty must drop nil data and metadata")
}

func TestDataAndMetadataKeepTheCallersValue(t *testing.T) {
	t.Parallel()

	type payload struct{ ID int }
	r := buildReply(ReplyOptions{}, WithDataOpt(payload{ID: 7}), WithMetadataOpt(map[string]int{"page": 2}))

	data, ok := r.Data.(payload)
	require.True(t, ok, "Reply.Data holds %T, want the caller's payload type", r.Data)
	require.Equal(t, 7, data.ID)
	require.Equal(t, map[string]int{"page": 2}, r.Metadata)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"ID":7},"metadata":{"page":2}}`, string(b))
}
