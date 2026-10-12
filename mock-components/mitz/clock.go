package mitzmock

import (
	"sync"
	"time"
)

// mockClock issues the timestamps of consent changes and of cleanup
// watermarks. Each is strictly later than every one before it, whatever the
// wall clock does, so a caller can order and fence notifications by them. That
// guarantee is this mock's; the national Mitz promises no such thing. A process
// restarted with its wall clock set back loses it until the clock has caught up.
type mockClock struct {
	mu   sync.Mutex
	last time.Time
	now  func() time.Time
}

func newMockClock() *mockClock { return &mockClock{now: time.Now} }

func (c *mockClock) next() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	// UTC and without the monotonic reading: the value is compared after a trip
	// through JSON, where only the wall time survives.
	reading := c.now().UTC().Round(0)
	if !reading.After(c.last) {
		reading = c.last.Add(time.Nanosecond)
	}
	c.last = reading
	return reading
}
