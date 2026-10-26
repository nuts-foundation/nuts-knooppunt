package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const eventHeartbeat = 15 * time.Second

func (c Config) startPatientRun(session *authSession, key string, switchPatient bool) error {
	if c.Runs == nil {
		acquire := c.Locks.Lock
		if switchPatient {
			acquire = c.Locks.Switch
		}
		if !acquire(key, lockOwner(session)) {
			return errPatientBusy
		}
		return nil
	}
	var err error
	start := func() {
		var id string
		if id, err = c.Runs.start(lockOwner(session), key, session.ExpiresAt, switchPatient); err == nil {
			c.Runs.bindSession(id, session.ID)
		}
	}
	if c.sessions != nil {
		if !c.sessions.storeIfLive(session.ID, start) {
			return errRunUnavailable
		}
	} else {
		start()
	}
	return err
}

func (c Config) withPatientRun(next func(http.ResponseWriter, *http.Request, *authSession)) func(http.ResponseWriter, *http.Request, *authSession) {
	return func(w http.ResponseWriter, r *http.Request, session *authSession) {
		id := c.Runs.current(lockOwner(session), r.PathValue("key"))
		if id != "" {
			sessionID, owner := session.ID, lockOwner(session)
			emit := func(event stepEvent) {
				c.sessions.storeIfLive(sessionID, func() { c.Runs.append(id, event) })
			}
			live := func() bool {
				owned := false
				return c.sessions.storeIfLive(sessionID, func() { owned = c.Runs.owns(id, owner) }) && owned
			}
			ctx := withEventCapture(r.Context(), emit, c.revealEventIdentifiers)
			ctx = withEventTraceCapture(ctx, c.eventTraces, live)
			r = r.WithContext(withEventAction(ctx, actionForRequest(r)))
		}
		next(w, r, session)
	}
}

func eventCursor(raw, id string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	runID, sequence, ok := strings.Cut(raw, ":")
	if !ok || runID != id || sequence == "" {
		return 0, errEventCursor
	}
	for _, char := range sequence {
		if char < '0' || char > '9' {
			return 0, errEventCursor
		}
	}
	after, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil {
		return 0, errEventCursor
	}
	return after, nil
}

func eventCrossOrigin(r *http.Request) bool {
	if crossSiteRequest(r) {
		return true
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil || secureCookies() {
			scheme = "https"
		}
		return err != nil || u.User != nil || u.Host != r.Host || u.Scheme != scheme
	}
	return false
}

func (c Config) handleEvents(w http.ResponseWriter, r *http.Request, session *authSession) {
	id, owner := r.PathValue("runId"), lockOwner(session)
	// Check ownership before interpreting a cursor, so another session cannot
	// distinguish an existing run from an unavailable one using malformed input.
	if !c.Runs.owns(id, owner) {
		http.Error(w, errRunUnavailable.Error(), http.StatusNotFound)
		return
	}
	after, err := eventCursor(r.Header.Get("Last-Event-ID"), id)
	if err != nil {
		http.Error(w, errEventCursor.Error(), http.StatusBadRequest)
		return
	}
	snapshot, err := c.Runs.snapshot(id, owner, after)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, errEventCursor) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	// net/http discards body writes for HEAD, so a stream would never see a
	// write error and would hold the connection until the run ends.
	if r.Method == http.MethodHead {
		return
	}
	controller := http.NewResponseController(w)
	write := func(frame string) bool {
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		if _, err := io.WriteString(w, frame); err != nil {
			return false
		}
		if err := controller.Flush(); err != nil {
			return false
		}
		// The deadline bounds a write, not the idle interval before the next
		// event. An expired HTTP/2 write deadline terminates the stream.
		err := controller.SetWriteDeadline(time.Time{})
		return err == nil || errors.Is(err, http.ErrNotSupported)
	}
	if !write("retry: 3000\n\n") {
		return
	}
	heartbeat := time.NewTicker(eventHeartbeat)
	defer heartbeat.Stop()
	initial := true
	var sentRevision uint64
	var recordChanged <-chan struct{}
	for {
		if snapshot.Gap && !write(fmt.Sprintf("event: replay-gap\ndata: {\"firstSeq\":%d}\n\n", snapshot.FirstSeq)) {
			return
		}
		sequence := max(snapshot.FirstSeq, after+1)
		for _, raw := range snapshot.Events {
			if !write(fmt.Sprintf("event: step\nid: %s:%d\ndata: %s\n\n", id, sequence, raw)) {
				return
			}
			sequence++
		}
		after = snapshot.LastSeq
		record, changed := c.recordSnapshotFor(owner, snapshot.Patient)
		recordChanged = changed
		// Also when no step is new: a retrieval stored after its last captured
		// call, or a consent change that was not notified, changes the record
		// without producing an event.
		if initial || len(snapshot.Events) > 0 || record.Revision != sentRevision {
			frame, err := json.Marshal(map[string]any{"lastSeq": after, "prsEvidence": c.eventTraces != nil, "record": record})
			if err != nil || !write("event: snapshot\ndata: "+string(frame)+"\n\n") {
				return
			}
			initial, sentRevision = false, record.Revision
		}
		expiry := time.NewTimer(time.Until(snapshot.ExpiresAt))
		select {
		case <-r.Context().Done():
			expiry.Stop()
			return
		case <-recordChanged:
		case <-snapshot.Changed:
		case <-expiry.C:
		case <-heartbeat.C:
			if !write(": keepalive\n\n") {
				expiry.Stop()
				return
			}
		}
		expiry.Stop()
		snapshot, err = c.Runs.snapshot(id, owner, after)
		if err != nil {
			write("event: run-ended\ndata: {}\n\n")
			return
		}
	}
}

// recordSnapshot is what an open record page compares itself with: the
// revision of what the record shows for the patient, and whether this session
// holds retrieved data for it. It is read when a snapshot is written, so it is
// the state now, also while an older action is replayed.
type recordSnapshot struct {
	Revision uint64 `json:"revision"`
	Copy     string `json:"copy"`
}

// recordSnapshotFor reads the two together and returns the channel that is
// closed at the patient's next revision advance. Together means inside the
// state mutex: every change to the retrieval store that the record shows is
// made in one of the state's transitions, so the copy read here is the copy of
// exactly this revision.
func (c Config) recordSnapshotFor(owner, patientKey string) (recordSnapshot, <-chan struct{}) {
	state := recordSnapshot{Copy: "none"}
	revision, changed := c.runtime.snapshot(patientKey, func() {
		held, removed := c.Retrievals.copyState(owner, patientKey)
		switch {
		case removed:
			state.Copy = "removed"
		case held:
			state.Copy = "held"
		}
	})
	state.Revision = revision
	return state, changed
}
