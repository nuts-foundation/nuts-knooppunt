package mitzmock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMockClock_NeverRepeatsAndNeverGoesBack(t *testing.T) {
	readings := []time.Time{
		time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC), // the same reading twice
		time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC), // the wall clock set back
		time.Date(2026, 10, 12, 14, 0, 1, 0, time.UTC),
	}
	i := 0
	clock := &mockClock{now: func() time.Time { r := readings[i]; i++; return r }}

	first, second, third, fourth := clock.next(), clock.next(), clock.next(), clock.next()
	require.Equal(t, readings[0], first)
	require.Equal(t, first.Add(time.Nanosecond), second)
	require.Equal(t, second.Add(time.Nanosecond), third)
	require.Equal(t, readings[3], fourth)
	require.True(t, second.After(first) && third.After(second) && fourth.After(third))
}

func TestMockClock_SurvivesAFormattingRoundTrip(t *testing.T) {
	clock := newMockClock()
	a, b := clock.next(), clock.next()
	parsedA, err := time.Parse(time.RFC3339Nano, a.Format(time.RFC3339Nano))
	require.NoError(t, err)
	parsedB, err := time.Parse(time.RFC3339Nano, b.Format(time.RFC3339Nano))
	require.NoError(t, err)
	require.True(t, parsedB.After(parsedA), "two timestamps stay ordered after being written and read")
}
