package httpserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

type errorReplyRecorder struct {
	Context
	status int
	reply  *Reply
}

func (r *errorReplyRecorder) Reply(code int, opts ...OptionReply) {
	r.status = code
	r.reply = BuildReply(ReplyOptions{}, opts...)
}
func (r *errorReplyRecorder) WithError(err error) OptionReply { return WithErrorOpt(err) }

func TestStatusForAndReplyError(t *testing.T) {
	for _, code := range []errorkit.ErrorCode{errorkit.ERR_CLIENT_NOT_FOUND, errorkit.ERR_VALIDATION, errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE} {
		err := errorkit.NewError(code).With(errorkit.WithReason("private failure"))
		wrapped := fmt.Errorf("boundary: %w", err)
		require.Equal(t, err.Metadata.HTTPStatus, StatusFor(wrapped))
		c := new(errorReplyRecorder)
		ReplyError(c, wrapped)
		require.Equal(t, err.Metadata.HTTPStatus, c.status)
		require.Equal(t, string(code), c.reply.Code)
		if err.Metadata.HTTPStatus >= 500 {
			require.NotContains(t, *c.reply.Error, "private failure")
		}
	}
	require.Equal(t, 200, StatusFor(nil))
	require.Equal(t, 500, StatusFor(errors.New("unknown")))
	invalid := errorkit.NewError(errorkit.ERR_INTERNAL)
	invalid.Metadata.HTTPStatus = 200
	require.Equal(t, 500, StatusFor(invalid))
	c := new(errorReplyRecorder)
	ReplyError(c, nil)
	require.Zero(t, c.status)
}

func TestReplyErrorSkipsTypedNilInJoinedErrors(t *testing.T) {
	var absent *errorkit.AppError
	expected := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	joined := errors.Join(absent, expected)
	c := new(errorReplyRecorder)
	require.NotPanics(t, func() { ReplyError(c, joined) })
	require.Equal(t, 404, c.status)
	require.Equal(t, string(expected.Code()), c.reply.Code)
	c = new(errorReplyRecorder)
	require.NotPanics(t, func() { ReplyError(c, absent) })
	require.Equal(t, 500, c.status)
	require.Equal(t, string(errorkit.ERR_INTERNAL), c.reply.Code)
}
