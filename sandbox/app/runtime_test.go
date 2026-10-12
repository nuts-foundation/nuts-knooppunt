package main

import (
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)

func notificationAt(decision string, at time.Time) consentNotification {
	return consentNotification{Decision: decision, ConsentTime: at, ReceivedAt: at.Add(time.Millisecond)}
}

// setProbe installs a test probe for the length of the test. The probe runs on
// the goroutine that reached the point, so it reports with t.Error, never with
// t.Fatal or require.
func setProbe(t *testing.T, state *runtimeState, probe func(point, patientKey string)) {
	t.Helper()
	state.probe.Store(&probe)
	t.Cleanup(func() { state.probe.Store(nil) })
}

// holdPoint holds the first goroutine that passes it until the test releases
// it, and lets every later one through. It is released when the test ends,
// whatever happened, so a failed assertion cannot strand the goroutine it holds
// and with it the server that goroutine belongs to. Create it after the
// fixture: cleanups run last-registered first.
type holdPoint struct {
	arrived chan struct{}
	proceed chan struct{}
	taken   atomic.Bool
	open    sync.Once
}

func newHoldPoint(t *testing.T) *holdPoint {
	t.Helper()
	h := &holdPoint{arrived: make(chan struct{}), proceed: make(chan struct{})}
	t.Cleanup(h.release)
	return h
}

func (h *holdPoint) pass() {
	if h.taken.CompareAndSwap(false, true) {
		close(h.arrived)
		<-h.proceed
	}
}

func (h *holdPoint) awaitArrival(t *testing.T) {
	t.Helper()
	select {
	case <-h.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing arrived at the hold point")
	}
}

func (h *holdPoint) release() { h.open.Do(func() { close(h.proceed) }) }

// notifiedPatients lists the keys a notification is recorded under, so a test
// can say that nothing was recorded for anybody, and not only for the patient it
// happened to look at.
func notifiedPatients(state *runtimeState) []string {
	state.stateMu.Lock()
	defer state.stateMu.Unlock()
	keys := []string{}
	for key := range state.notifications {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestRuntime_AcceptsOnlyWhatIsLaterThanTheWatermark(t *testing.T) {
	state := newRuntimeState()
	applied := 0
	apply := func() { applied++ }

	accepted, _ := state.accept("anna", notificationAt("deny", t0.Add(2*time.Second)), apply)
	require.True(t, accepted)
	accepted, _ = state.accept("anna", notificationAt("deny", t0.Add(2*time.Second)), apply)
	require.False(t, accepted, "the same dateTime again is a duplicate")
	accepted, _ = state.accept("anna", notificationAt("permit", t0.Add(time.Second)), apply)
	require.False(t, accepted, "an older one was overtaken")
	accepted, _ = state.accept("anna", notificationAt("permit", t0.Add(3*time.Second)), apply)
	require.True(t, accepted)

	require.Equal(t, 2, applied, "apply runs for an accepted notification and for no other")
	latest, ok := state.notification("anna")
	require.True(t, ok)
	require.Equal(t, "permit", latest.Decision)
	require.Equal(t, uint64(2), state.receivedCount("anna"))
	_, other := state.notification("bram")
	require.False(t, other, "one patient's notification is not another's")
}

func TestRuntime_AResetRaisesTheWatermarkAndKeepsIt(t *testing.T) {
	state := newRuntimeState()
	state.accept("anna", notificationAt("deny", t0), func() {})
	state.noteMockWatermark("", t0.Add(10*time.Second))
	cleared := false
	state.reset("anna", "999911120", func() { cleared = true })

	require.True(t, cleared)
	_, ok := state.notification("anna")
	require.False(t, ok, "the reset clears what the record shows")

	// A change made before the reset, whose notification is only handled now.
	accepted, _ := state.accept("anna", notificationAt("deny", t0.Add(5*time.Second)), func() {})
	require.False(t, accepted, "it is not later than the reset's watermark, however late it arrives")
	accepted, _ = state.accept("anna", notificationAt("deny", t0.Add(11*time.Second)), func() {})
	require.True(t, accepted, "a change made after the reset is")
}

func TestRuntime_WithoutAMockWatermarkAResetUsesWhatTheSandboxSaw(t *testing.T) {
	state := newRuntimeState()
	state.noteIssued("anna", t0.Add(4*time.Second)) // from a control response
	state.reset("anna", "999911120", nil)

	accepted, _ := state.accept("anna", notificationAt("deny", t0.Add(4*time.Second)), func() {})
	require.False(t, accepted)
	accepted, _ = state.accept("anna", notificationAt("deny", t0.Add(4*time.Second+time.Nanosecond)), func() {})
	require.True(t, accepted)
}

func TestRuntime_APerPatientMockWatermarkCoversThatPatientOnly(t *testing.T) {
	state := newRuntimeState()
	state.noteMockWatermark("999911120", t0.Add(10*time.Second))
	state.reset("anna", "999911120", nil)
	state.reset("bram", "999911132", nil)

	accepted, _ := state.accept("anna", notificationAt("deny", t0.Add(5*time.Second)), func() {})
	require.False(t, accepted)
	accepted, _ = state.accept("bram", notificationAt("deny", t0.Add(5*time.Second)), func() {})
	require.True(t, accepted)
}

func TestRuntime_TheResetCounterOnlyMovesOnAReset(t *testing.T) {
	state := newRuntimeState()
	_, before := state.accept("anna", notificationAt("deny", t0), func() {})
	require.Equal(t, state.resetsOf("anna"), before)
	state.reset("anna", "999911120", nil)
	require.Equal(t, before+1, state.resetsOf("anna"))
	require.Zero(t, state.resetsOf("bram"))
}

func TestRuntime_TheRevisionOnlyEverIncreasesAndWakesItsWatchers(t *testing.T) {
	state := newRuntimeState()
	first, changed := state.recordState("anna")
	select {
	case <-changed:
		t.Fatal("nothing changed yet")
	default:
	}
	again, _ := state.recordState("anna")
	require.Equal(t, first, again, "reading the revision does not change it")

	state.advance("anna")
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("an advance must wake whoever watches the patient")
	}
	second, later := state.recordState("anna")
	require.Greater(t, second, first)
	select {
	case <-later:
		t.Fatal("the new channel is open until the next advance")
	default:
	}

	state.accept("anna", notificationAt("deny", t0), func() {})
	third, _ := state.recordState("anna")
	require.Greater(t, third, second, "an accepted notification advances it")
	state.reset("anna", "999911120", nil)
	fourth, _ := state.recordState("anna")
	require.Greater(t, fourth, third, "a reset advances it and never sets it back")
	otherBefore, _ := state.recordState("bram")
	require.Zero(t, otherBefore, "another patient's revision is its own")
}

func TestRuntime_PendingPairsByChangeIDAndNothingElse(t *testing.T) {
	state := newRuntimeState()
	event := stepEvent{CallID: "notification"}
	require.False(t, state.handOff("anna", "change-a", event), "no pending action")

	state.openPending("anna", pendingConsent{actionID: "action-a", runID: "run-1", changeID: "change-a"})
	require.False(t, state.handOff("anna", "change-b", event), "the same patient and run, another change")
	require.False(t, state.handOff("bram", "change-a", event), "another patient")
	require.True(t, state.handOff("anna", "change-a", event))

	require.Empty(t, state.closePending("anna", "change-b"), "an action only closes its own entry")
	taken := state.closePending("anna", "change-a")
	require.Len(t, taken, 1)
	require.Equal(t, "notification", taken[0].CallID)
	require.False(t, state.handOff("anna", "change-a", event), "after the entry closed an event is on its own")
	require.Empty(t, state.closePending("anna", "change-a"), "closing twice hands nothing out twice")
}

func TestRuntime_ConsentLockIsOnePerPatient(t *testing.T) {
	state := newRuntimeState()
	require.Same(t, state.consentLock("anna"), state.consentLock("anna"))
	require.NotSame(t, state.consentLock("anna"), state.consentLock("bram"))
}

// A snapshot's revision and what it reads of the retrieval store belong together
// only because the read and every change run under one mutex. Each callback
// looks at that mutex from the inside.
func TestRuntime_ReadsAndTransitionsShareOneExclusion(t *testing.T) {
	state := newRuntimeState()
	ran := 0
	inside := func(what string) func() {
		return func() {
			ran++
			if state.stateMu.TryLock() {
				state.stateMu.Unlock()
				t.Errorf("%s runs outside the state mutex", what)
			}
		}
	}
	state.accept("anna", notificationAt("deny", t0), inside("a notification's apply"))
	state.change("anna", inside("a retrieval's change"))
	state.reset("anna", "999911120", inside("a reset's clear"))
	state.snapshot("anna", inside("a snapshot's read"))
	require.Equal(t, 4, ran)
}

func TestRuntime_AChangeWakesItsWatchersOnlyWhenItIsComplete(t *testing.T) {
	state := newRuntimeState()
	before, changed := state.recordState("anna")
	state.change("anna", func() {
		select {
		case <-changed:
			t.Error("the watchers were woken before the change was applied")
		default:
		}
	})
	after, _ := state.recordState("anna")
	require.Equal(t, before+1, after)
	select {
	case <-changed:
	default:
		t.Fatal("a change must wake whoever watches the patient")
	}
}

// A notification's watermark check and its commit are one step. The probe sits
// at the boundary between them: the mutex is held there, and a newer
// notification that arrives at that moment is committed after the older one.
func TestRuntime_ANewerNotificationCannotGetBetweenAnOlderOnesCheckAndItsCommit(t *testing.T) {
	state := newRuntimeState()
	competing := make(chan bool, 1)
	var once sync.Once
	setProbe(t, state, func(point, _ string) {
		if point != probeAcceptCommitting {
			return
		}
		once.Do(func() {
			if state.stateMu.TryLock() {
				state.stateMu.Unlock()
				t.Error("the state mutex is free between an accepted notification's check and its commit")
			}
			go func() {
				accepted, _ := state.accept("anna", notificationAt("permit", t0.Add(time.Second)), nil)
				competing <- accepted
			}()
		})
	})

	accepted, _ := state.accept("anna", notificationAt("deny", t0), nil)

	require.True(t, accepted)
	select {
	case newer := <-competing:
		require.True(t, newer, "the newer one is accepted, after the older one committed")
	case <-time.After(5 * time.Second):
		t.Fatal("the competing notification was never handled")
	}
	latest, _ := state.notification("anna")
	require.Equal(t, "permit", latest.Decision, "the newer notification stands")
	require.Equal(t, uint64(2), state.receivedCount("anna"))
}

// The reset counter guards the publication of an event that was accepted before
// a reset. The guard is only worth something if no reset can land between the
// check of the counter and the publication it allows.
func TestRuntime_AResetAndAPublicationExcludeEachOther(t *testing.T) {
	state := newRuntimeState()
	resets := state.resetsOf("anna")

	published := state.publishUnlessReset("anna", resets, func() {
		if state.publication.TryLock() {
			state.publication.Unlock()
			t.Error("a reset could make its transition between the check of the counter and the publication")
		}
		if !state.stateMu.TryLock() {
			t.Error("the state mutex is held while publishing, which takes the session, run and registry locks")
			return
		}
		state.stateMu.Unlock()
	})
	require.True(t, published)

	state.reset("anna", "999911120", func() {
		if state.publication.TryRLock() {
			state.publication.RUnlock()
			t.Error("a publication could check the counter while the reset is in its transition")
		}
	})
	ran := false
	require.False(t, state.publishUnlessReset("anna", resets, func() { ran = true }), "the patient was reset since the counter was read")
	require.False(t, ran)
	require.True(t, state.publishUnlessReset("bram", state.resetsOf("bram"), func() {}), "one patient's reset is not another's")
}

// A test may deliver a notification from this probe, which takes the state mutex
// and the fence. So the probe has to fire with neither held, and the test looks
// at both from inside it.
func TestRuntime_AProbeSeesTheEndOfAResetWithNothingHeld(t *testing.T) {
	state := newRuntimeState()
	var seen []string
	setProbe(t, state, func(point, patientKey string) {
		seen = append(seen, point+" "+patientKey)
		if !state.publication.TryLock() {
			t.Error("the probe fires while the reset still holds the fence")
		} else {
			state.publication.Unlock()
		}
		if !state.stateMu.TryLock() {
			t.Error("the probe fires while the reset still holds the state mutex")
		} else {
			state.stateMu.Unlock()
		}
	})
	state.reset("anna", "999911120", nil)
	require.Equal(t, []string{probeResetApplied + " anna"}, seen)
}
