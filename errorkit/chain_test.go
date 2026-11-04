package errorkit_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
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
		{name: "joined typed nil before AppError", err: errors.Join(nilAppErr, appErr), want: appErr, wantOK: true},
		{name: "nested joined typed nil before AppError", err: fmt.Errorf("outer: %w", errors.Join(errors.Join(nilAppErr, errors.New("other")), fmt.Errorf("inner: %w", appErr))), want: appErr, wantOK: true},
		{name: "joined typed nil without AppError", err: errors.Join(nilAppErr, errors.New("other"))},
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
		{name: "joined after another AppError", err: errors.Join(errorkit.NewError(errorkit.ERR_VALIDATION), inner), code: errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, want: true},
		{name: "joined inside a wrapped AppError", err: errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(errors.Join(errors.New("a"), fmt.Errorf("b: %w", inner)))), code: errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, want: true},
		{name: "joined without the code", err: errors.Join(errors.New("a"), errorkit.NewError(errorkit.ERR_VALIDATION)), code: errorkit.ERR_INTERNAL, want: false},
		{name: "typed nil AppError", err: (*errorkit.AppError)(nil), code: errorkit.ERR_UNKNOWN, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorkit.HasCode(tc.err, tc.code); got != tc.want {
				t.Errorf("HasCode(%v, %s) = %v, want %v", tc.err, tc.code, got, tc.want)
			}
		})
	}
}
