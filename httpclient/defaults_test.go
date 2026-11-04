package httpclient

import (
	"testing"
	"time"
)

func TestDefaultDialTimeoutIsShort(t *testing.T) {
	t.Parallel()

	if DefaultDialTimeout > 10*time.Second {
		t.Fatalf("DefaultDialTimeout = %s, want at most 10s", DefaultDialTimeout)
	}
	if got := DefaultConfig().DialTimeout; got != DefaultDialTimeout {
		t.Fatalf("DefaultConfig().DialTimeout = %s, want DefaultDialTimeout (%s)", got, DefaultDialTimeout)
	}
}
