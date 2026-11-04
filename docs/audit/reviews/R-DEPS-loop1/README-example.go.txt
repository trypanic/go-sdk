package main

import (
	"context"
	"errors"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/logger"
)

func main() {
	_, cleanup := logger.Init("my-service", "0.1.0")
	defer cleanup.Close()

	ctx := context.Background()
	if err := fetchOrder(ctx); err != nil {
		// Wrap once at the boundary; keep an existing *errorkit.AppError as is.
		appErr, ok := errorkit.As(err)
		if !ok {
			appErr = errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
		}
		logger.ErrorCtx(ctx, appErr, "fetch order failed")
		return
	}
	logger.LogInfoWithTrace(ctx, "order fetched")
}

func fetchOrder(context.Context) error { return errors.New("upstream unavailable") }
