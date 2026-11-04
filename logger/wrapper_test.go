package logger

import (
	"errors"
	"testing"
)

func TestErrorBeforeInitDoesNotPanic(t *testing.T) {
	previousGlobal := global.Load()
	global.Store(nil)
	t.Cleanup(func() {
		global.Store(previousGlobal)
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Error() panicked before Init(): %v", r)
		}
	}()

	err := errors.New("startup failed")
	if got := Error(err, "failed to start"); got != err {
		t.Fatalf("Error() = %v, want original error %v", got, err)
	}
}
