package errorkit_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestAsFindsAppErrorInWrappedChain(t *testing.T) {
	appErr := errorkit.NewError(errorkit.ERR_VALIDATION)
	var nilAppErr *errorkit.AppError

	cases := []struct {
		name   string
		err    error
		want   *errorkit.AppError
		wantOK bool
	}{
		{name: "nil", err: nil},
		{name: "plain error", err: errors.New("boom")},
		{name: "direct", err: appErr, want: appErr, wantOK: true},
		{name: "wrapped once", err: fmt.Errorf("ctx: %w", appErr), want: appErr, wantOK: true},
		{name: "wrapped twice", err: fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", appErr)), want: appErr, wantOK: true},
		{name: "joined", err: errors.Join(errors.New("other"), appErr), want: appErr, wantOK: true},
		{name: "typed nil", err: nilAppErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := errorkit.As(tc.err)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("As(%v) = (%p, %v), want (%p, %v)", tc.err, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestHasCode(t *testing.T) {
	inner := errorkit.NewError(errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL)
	outer := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(fmt.Errorf("query: %w", inner)))

	cases := []struct {
		name string
		err  error
		code errorkit.ErrorCode
		want bool
	}{
		{name: "nil", err: nil, code: errorkit.ERR_INTERNAL, want: false},
		{name: "plain error", err: errors.New("boom"), code: errorkit.ERR_INTERNAL, want: false},
		{name: "direct match", err: inner, code: errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, want: true},
		{name: "direct mismatch", err: inner, code: errorkit.ERR_INTERNAL, want: false},
		{name: "wrapped with %w", err: fmt.Errorf("svc: %w", inner), code: errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, want: true},
		{name: "outer code", err: fmt.Errorf("svc: %w", outer), code: errorkit.ERR_INTERNAL, want: true},
		{name: "code of a nested AppError", err: fmt.Errorf("svc: %w", outer), code: errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, want: true},
		{name: "absent code", err: outer, code: errorkit.ERR_VALIDATION, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorkit.HasCode(tc.err, tc.code); got != tc.want {
				t.Errorf("HasCode(%v, %s) = %v, want %v", tc.err, tc.code, got, tc.want)
			}
		})
	}
}
