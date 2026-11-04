package backoff

import (
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrowSaturatesAndHandlesInvalidMultiplier(t *testing.T) {
	for _, tc := range []struct {
		current, limit time.Duration
		multiplier     float64
		want           time.Duration
	}{
		{time.Second, 10 * time.Second, 2, 2 * time.Second},
		{6 * time.Second, 10 * time.Second, 2, 10 * time.Second},
		{time.Duration(math.MaxInt64/2 + 1), time.Duration(math.MaxInt64), 2, time.Duration(math.MaxInt64)},
		{time.Second, 10 * time.Second, math.Inf(1), 10 * time.Second},
		{time.Second, 10 * time.Second, math.NaN(), 2 * time.Second},
	} {
		require.Equal(t, tc.want, Grow(tc.current, tc.limit, tc.multiplier))
	}
}

func TestFullJitterStaysBelowCap(t *testing.T) {
	for _, cap := range []time.Duration{0, -1, 1, time.Second, time.Duration(math.MaxInt64)} {
		for range 20 {
			delay := Jitter(cap)
			require.GreaterOrEqual(t, delay, time.Duration(0))
			if cap > 0 {
				require.Less(t, delay, cap)
			} else {
				require.Zero(t, delay)
			}
		}
	}
}

func TestScheduleAttemptBudgetAndReset(t *testing.T) {
	schedule := New(Config{Initial: time.Second, Max: 3 * time.Second, Attempts: 4, Multiplier: 2})
	for _, cap := range []time.Duration{time.Second, 2 * time.Second, 3 * time.Second} {
		delay := schedule.NextBackOff()
		require.GreaterOrEqual(t, delay, time.Duration(0))
		require.Less(t, delay, cap)
	}
	require.Equal(t, Stop, schedule.NextBackOff())
	schedule.Reset()
	require.NotEqual(t, Stop, schedule.NextBackOff())
}

func TestScheduleElapsedBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		schedule := New(Config{Attempts: 3, MaxElapsedTime: time.Minute})
		time.Sleep(time.Minute)
		require.Equal(t, Stop, schedule.NextBackOff())
	})
}
