package logger

import (
	"sync"
	"testing"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func TestInitConcurrentWithSDKLogging(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	previous := global.Load()
	previousLevel, previousFormat, previousZlog := zerolog.GlobalLevel(), zerolog.TimeFieldFormat, zlog.Logger
	t.Cleanup(func() {
		global.Store(previous)
		zerolog.SetGlobalLevel(previousLevel)
		zerolog.TimeFieldFormat = previousFormat
		zlog.Logger = previousZlog
	})
	_, cleanup := Init("test", "1")
	t.Cleanup(func() {
		if err := cleanup.Close(); err != nil {
			t.Errorf("close logger: %v", err)
		}
	})
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 25 {
			_, c := Init("test", "1")
			_ = c.Close()
		}
	})
	wg.Go(func() {
		for range 25 {
			Info("concurrent global log")
		}
	})
	wg.Wait()
}
