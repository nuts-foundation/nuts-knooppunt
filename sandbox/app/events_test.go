package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func eventStoreAt(t *testing.T) (*runStore, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	locks := NewRegistry()
	locks.now = func() time.Time { return now }
	store := newRunStore(locks)
	store.now = func() time.Time { return now }
	return store, &now
}

func decodedSteps(t *testing.T, snapshot runSnapshot) []stepEvent {
	t.Helper()
	var events []stepEvent
	for _, raw := range snapshot.Events {
		var event stepEvent
		require.NoError(t, json.Unmarshal(raw, &event))
		events = append(events, event)
	}
	return events
}

func TestRunStore_ConcurrentPublicationIsOrderedAndIsolated(t *testing.T) {
	store, now := eventStoreAt(t)
	first, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	second, err := store.start("two", "pool-02", now.Add(time.Hour), true)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() { store.append(first, stepEvent{GF: "localization", Outcome: "ok"}) })
		wg.Go(func() { store.append(second, stepEvent{GF: "consent", Outcome: "deny"}) })
	}
	wg.Wait()
	for _, pair := range []struct{ id, owner, gf string }{{first, "one", "localization"}, {second, "two", "consent"}} {
		snapshot, err := store.snapshot(pair.id, pair.owner, 0)
		require.NoError(t, err)
		events := decodedSteps(t, snapshot)
		require.Len(t, events, 40)
		for i, event := range events {
			require.Equal(t, uint64(i+1), event.Seq)
			require.Equal(t, pair.id, event.RunID)
			require.Equal(t, pair.gf, event.GF)
		}
	}
	_, err = store.snapshot(first, "two", 0)
	require.ErrorIs(t, err, errRunUnavailable)
}

func TestRunStore_ReplayWindowAndFutureCursor(t *testing.T) {
	store, now := eventStoreAt(t)
	store.maxEvents = 3
	id, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	for range 5 {
		require.True(t, store.append(id, stepEvent{GF: "exchange", Outcome: "ok"}))
	}
	window, err := store.snapshot(id, "one", 0)
	require.NoError(t, err)
	require.True(t, window.Gap)
	require.Equal(t, uint64(3), window.FirstSeq)
	require.Len(t, window.Events, 3)
	window.Events[0][0] = '!'
	resumed, err := store.snapshot(id, "one", 3)
	require.NoError(t, err)
	require.False(t, resumed.Gap)
	require.Equal(t, uint64(4), decodedSteps(t, resumed)[0].Seq)
	all, err := store.snapshot(id, "one", 0)
	require.NoError(t, err)
	require.Equal(t, uint64(3), decodedSteps(t, all)[0].Seq, "snapshots must not share writable retained bytes")
	_, err = store.snapshot(id, "one", 6)
	require.ErrorIs(t, err, errEventCursor)
}

func TestRunStore_ActivityRenewsButReadersDoNot(t *testing.T) {
	store, now := eventStoreAt(t)
	id, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	*now = now.Add(14 * time.Minute)
	require.True(t, store.append(id, stepEvent{GF: "addressing"}))
	*now = now.Add(2 * time.Minute)
	require.True(t, store.locks.HeldBy("anna", "one"))
	_, err = store.snapshot(id, "one", 0)
	require.NoError(t, err)
	*now = now.Add(14 * time.Minute)
	_, err = store.snapshot(id, "one", 0)
	require.ErrorIs(t, err, errRunUnavailable)
	require.False(t, store.locks.IsLocked("anna"))
	require.False(t, store.append(id, stepEvent{GF: "exchange"}))
}

func TestRunStore_SessionExpiryCapsRetention(t *testing.T) {
	store, now := eventStoreAt(t)
	id, err := store.start("one", "anna", now.Add(time.Minute), true)
	require.NoError(t, err)
	*now = now.Add(time.Minute)
	require.False(t, store.append(id, stepEvent{GF: "exchange"}))
	require.Empty(t, store.current("one", "anna"))
	require.False(t, store.locks.IsLocked("anna"))
}

func TestRunStore_ActivityCannotOutliveSession(t *testing.T) {
	store, now := eventStoreAt(t)
	expires := now.Add(5 * time.Minute)
	id, err := store.start("one", "anna", expires, true)
	require.NoError(t, err)
	*now = now.Add(4 * time.Minute)
	require.True(t, store.append(id, stepEvent{GF: "exchange", Outcome: "ok"}))
	snapshot, err := store.snapshot(id, "one", 0)
	require.NoError(t, err)
	require.Equal(t, expires, snapshot.ExpiresAt)
	*now = expires
	_, err = store.snapshot(id, "one", 0)
	require.ErrorIs(t, err, errRunUnavailable)
	require.False(t, store.locks.IsLocked("anna"))
}

func TestRunStore_OversizedEventDropsBodiesBeforeRetention(t *testing.T) {
	store, now := eventStoreAt(t)
	id, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	identifiers := make([]any, 8)
	for i := range identifiers {
		identifiers[i] = map[string]any{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "999900006"}
	}
	entries := make([]any, 32)
	for i := range entries {
		entries[i] = map[string]any{"resource": map[string]any{"resourceType": "Patient", "identifier": identifiers}}
	}
	input, err := json.Marshal(map[string]any{"resourceType": "Bundle", "type": "searchset", "entry": entries})
	require.NoError(t, err)
	body := eventBody(input, "application/fhir+json", false)
	require.NotNil(t, body)
	projection, err := json.Marshal(body)
	require.NoError(t, err)
	require.Greater(t, len(projection), maxRetainedEventBytes, "exercise the cap with a real allowlisted projection")
	event := stepEvent{
		GF: "exchange", Outcome: "ok",
		Request:  eventRequest{Method: "POST", Path: "/fhir/Patient/_search", Body: body},
		Response: eventResponse{Status: 200, Body: body},
	}
	require.True(t, store.append(id, event))
	snapshot, err := store.snapshot(id, "one", 0)
	require.NoError(t, err)
	require.Len(t, snapshot.Events, 1)
	require.LessOrEqual(t, len(snapshot.Events[0]), maxRetainedEventBytes)
	retained := decodedSteps(t, snapshot)[0]
	require.Equal(t, "/fhir/Patient/_search", retained.Request.Path)
	require.Equal(t, 200, retained.Response.Status)
	require.Nil(t, retained.Request.Body)
	require.Nil(t, retained.Response.Body)

	event.Request.Path = strings.Repeat("x", maxRetainedEventBytes)
	require.False(t, store.append(id, event), "metadata that cannot fit must also be rejected")
	require.True(t, store.append(id, stepEvent{GF: "exchange", Outcome: "ok"}))
	snapshot, err = store.snapshot(id, "one", 0)
	require.NoError(t, err)
	require.Len(t, snapshot.Events, 2)
	require.Equal(t, uint64(2), snapshot.LastSeq, "rejected events must not consume sequence numbers")
}

func TestRunStore_ResumeSwitchAndDeniedSwitch(t *testing.T) {
	store, now := eventStoreAt(t)
	id, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	same, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	require.Equal(t, id, same)
	_, err = store.start("two", "pool-02", now.Add(time.Hour), true)
	require.NoError(t, err)
	_, err = store.start("one", "pool-02", now.Add(time.Hour), true)
	require.ErrorIs(t, err, errPatientBusy)
	require.Equal(t, id, store.current("one", "anna"))
	before, err := store.snapshot(id, "one", 0)
	require.NoError(t, err)
	other, err := store.start("one", "pool-03", now.Add(time.Hour), true)
	require.NoError(t, err)
	require.NotEqual(t, id, other)
	require.Empty(t, store.current("one", "anna"))
	require.False(t, store.locks.IsLocked("anna"))
	select {
	case <-before.Changed:
	default:
		t.Fatal("switch did not wake the old stream")
	}
}

func TestRunStore_ClearRejectsLateResponses(t *testing.T) {
	for _, mode := range []string{"owner", "patient", "all", "release"} {
		t.Run(mode, func(t *testing.T) {
			store, now := eventStoreAt(t)
			id, err := store.start("one", "anna", now.Add(time.Hour), true)
			require.NoError(t, err)
			other, err := store.start("two", "pool-02", now.Add(time.Hour), true)
			require.NoError(t, err)
			snapshot, err := store.snapshot(id, "one", 0)
			require.NoError(t, err)
			switch mode {
			case "owner":
				store.clearOwner("one")
			case "patient":
				store.clearPatient("anna")
			case "all":
				store.clear()
			case "release":
				store.release("one", "anna")
			}
			require.False(t, store.append(id, stepEvent{GF: "exchange"}))
			_, err = store.snapshot(id, "one", 0)
			require.ErrorIs(t, err, errRunUnavailable)
			select {
			case <-snapshot.Changed:
			default:
				t.Fatal("cleanup did not wake the stream")
			}
			if mode != "all" {
				_, err = store.snapshot(other, "two", 0)
				require.NoError(t, err)
			}
		})
	}
}

func TestRunStore_CapacityDoesNotEvictLiveRuns(t *testing.T) {
	store, now := eventStoreAt(t)
	store.maxRuns = 2
	for i := range 2 {
		_, err := store.start(fmt.Sprint(i), fmt.Sprint(i), now.Add(time.Hour), true)
		require.NoError(t, err)
	}
	_, err := store.start("new", "new", now.Add(time.Hour), true)
	require.ErrorIs(t, err, errRunCapacity)
	require.False(t, store.locks.IsLocked("new"))
	_, err = store.start("0", "another-patient", now.Add(time.Hour), true)
	require.NoError(t, err, "switching replaces an existing run even at capacity")
	*now = now.Add(16 * time.Minute)
	_, err = store.start("new", "new", now.Add(time.Hour), true)
	require.NoError(t, err)
}

func TestRunStore_LockLossInvalidatesRun(t *testing.T) {
	store, now := eventStoreAt(t)
	id, err := store.start("one", "anna", now.Add(time.Hour), true)
	require.NoError(t, err)
	store.locks.Release("anna", "one")
	require.True(t, store.locks.Lock("anna", "two"))
	require.False(t, store.append(id, stepEvent{GF: "exchange"}))
	require.True(t, store.locks.HeldBy("anna", "two"), "cleanup must not release a different owner's lock")
}
