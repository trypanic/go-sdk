// Package envs provides a thin wrapper around caarlos0/env for loading
// service configuration from environment variables into typed structs.
package envs

import (
	"github.com/caarlos0/env/v11"
	"github.com/trypanic/go-sdk/errorkit"
)

func NewLoader(c any) error {
	if err := env.Parse(c); err != nil {
		return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("failed to parse environment variables: %v", err),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}
