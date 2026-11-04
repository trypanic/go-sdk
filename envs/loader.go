// Package envs provides a thin wrapper around caarlos0/env for loading
// service configuration from environment variables into typed structs.
package envs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/trypanic/go-sdk/errorkit"
)

// NewLoader parses environment variables into c, a non-nil pointer to a
// struct tagged for caarlos0/env. On failure it returns an
// ERR_SYSTEM_CONFIG_INVALID *errorkit.AppError whose text names the
// offending variables and fields but never the raw values; the original
// caarlos0/env error stays reachable with errors.As.
func NewLoader(c any) error {
	if err := env.Parse(c); err != nil {
		return configError(err)
	}
	return nil
}

// configError wraps a caarlos0/env error in a value-free AppError.
func configError(err error) error {
	safe := &valueFreeError{msg: describe(err), cause: err}
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
		errorkit.WithReason("failed to parse environment variables: %s", safe.msg),
		errorkit.WithWrapped(safe),
	)
}

// valueFreeError prints a message that carries no environment values while
// keeping the original error in its chain for errors.Is/As.
type valueFreeError struct {
	msg   string
	cause error
}

func (e *valueFreeError) Error() string { return "env: " + e.msg }
func (e *valueFreeError) Unwrap() error { return e.cause }

// describe builds a message from the caarlos0/env error types that names
// variables, fields and types only. The library's own messages for
// conversion failures quote the offending value, so those are rebuilt.
func describe(err error) string {
	errs := []error{err}
	if agg, ok := errors.AsType[env.AggregateError](err); ok {
		errs = agg.Errors
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, describeOne(e))
	}
	return strings.Join(parts, "; ")
}

func describeOne(err error) string {
	switch e := err.(type) {
	case env.ParseError:
		return fmt.Sprintf("parse error on field %q of type %q", e.Name, e.Type)
	case env.ParseValueError:
		return e.Msg
	case env.LoadFileContentError:
		return fmt.Sprintf("could not load file content from variable %s", e.Key)
	case env.VarIsNotSetError, env.EmptyVarError, env.NotStructPtrError,
		env.NoParserError, env.NoSupportedTagOptionError:
		// These messages contain only names, types and tag options.
		return e.Error()
	default:
		return fmt.Sprintf("unexpected %T error", err)
	}
}
