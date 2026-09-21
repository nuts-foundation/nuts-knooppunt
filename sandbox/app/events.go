package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

const (
	defaultEventTTL       = 15 * time.Minute
	defaultMaxRuns        = 32
	defaultMaxRunEvents   = 256
	maxRetainedEventBytes = 16 * 1024
)

var (
	errRunUnavailable = errors.New("run is unavailable")
	errPatientBusy    = errors.New("patient is in use by another demo")
	errRunCapacity    = errors.New("too many active demo runs")
	errEventCursor    = errors.New("invalid event cursor")
)

type runScope struct{ owner, patient string }

type eventRun struct {
	id             string
	scope          runScope
	expiresAt      time.Time
	sessionExpires time.Time
	seq            uint64
	events         []json.RawMessage
	changed        chan struct{}
	sources        *sourceSelection
}

type runSnapshot struct {
	Events    []json.RawMessage
	FirstSeq  uint64
	LastSeq   uint64
	Gap       bool
	Changed   <-chan struct{}
	ExpiresAt time.Time
}

type runStore struct {
	mu        sync.Mutex
	locks     *Registry
	runs      map[string]*eventRun
	currentID map[runScope]string
	now       func() time.Time
	ttl       time.Duration
	maxRuns   int
	maxEvents int
}

func newRunStore(locks *Registry) *runStore {
	return &runStore{
		locks: locks, runs: make(map[string]*eventRun), currentID: make(map[runScope]string),
		now: time.Now, ttl: defaultEventTTL, maxRuns: defaultMaxRuns, maxEvents: defaultMaxRunEvents,
	}
}

func (s *runStore) start(owner, patient string, sessionExpires time.Time, switchPatient bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	if !sessionExpires.After(now) {
		return "", errRunUnavailable
	}
	scope := runScope{owner, patient}
	id := s.currentID[scope]
	count := len(s.runs)
	if switchPatient {
		for _, run := range s.runs {
			if run.scope.owner == owner && run.scope != scope {
				count--
			}
		}
	}
	if id == "" && count >= s.maxRuns {
		return "", errRunCapacity
	}
	acquire := s.locks.Lock
	if switchPatient {
		acquire = s.locks.Switch
	}
	if !acquire(patient, owner) {
		return "", errPatientBusy
	}
	if switchPatient {
		for _, run := range s.runs {
			if run.scope.owner == owner && run.scope != scope {
				s.remove(run)
			}
		}
	}
	run := s.runs[id]
	if run == nil {
		id = randomURLSafe()
		run = &eventRun{id: id, scope: scope, changed: make(chan struct{})}
		s.runs[id], s.currentID[scope] = run, id
	}
	run.sessionExpires = sessionExpires
	run.expiresAt = earlier(now.Add(s.ttl), sessionExpires)
	return id, nil
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *runStore) live(run *eventRun, now time.Time) bool {
	return run.expiresAt.After(now) && s.locks.HeldBy(run.scope.patient, run.scope.owner)
}

// The order is session store -> run store -> lock registry. No method in this
// store calls back into a session store or an event producer.
func (s *runStore) prune(now time.Time) {
	for _, run := range s.runs {
		if !s.live(run, now) {
			s.remove(run)
		}
	}
}

func (s *runStore) remove(run *eventRun) {
	delete(s.runs, run.id)
	delete(s.currentID, run.scope)
	s.locks.Release(run.scope.patient, run.scope.owner)
	close(run.changed)
}

func (s *runStore) current(owner, patient string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(s.now())
	return s.currentID[runScope{owner, patient}]
}

// append accepts only the capture middleware's sanitized projection. Serializing
// here assigns publication order and severs all mutable producer references.
func (s *runStore) append(id string, event stepEvent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	run := s.runs[id]
	if run == nil {
		return false
	}
	event.RunID, event.Seq = id, run.seq+1
	raw, err := json.Marshal(event)
	if err != nil {
		return false
	}
	if len(raw) > maxRetainedEventBytes {
		event.Request.Body, event.Response.Body = nil, nil
		raw, err = json.Marshal(event)
		if err != nil || len(raw) > maxRetainedEventBytes {
			return false
		}
	}
	if !s.locks.Refresh(run.scope.patient, run.scope.owner) {
		s.remove(run)
		return false
	}
	run.expiresAt = earlier(now.Add(s.ttl), run.sessionExpires)
	run.seq++
	if len(run.events) == s.maxEvents {
		copy(run.events, run.events[1:])
		run.events[len(run.events)-1] = raw
	} else {
		run.events = append(run.events, raw)
	}
	close(run.changed)
	run.changed = make(chan struct{})
	return true
}

func (s *runStore) owns(id, owner string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(s.now())
	run := s.runs[id]
	return run != nil && run.scope.owner == owner
}

func (s *runStore) snapshot(id, owner string, after uint64) (runSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(s.now())
	run := s.runs[id]
	if run == nil || run.scope.owner != owner {
		return runSnapshot{}, errRunUnavailable
	}
	if after > run.seq {
		return runSnapshot{}, errEventCursor
	}
	first := run.seq - uint64(len(run.events)) + 1
	result := runSnapshot{
		FirstSeq: first, LastSeq: run.seq, Gap: after+1 < first,
		Changed: run.changed, ExpiresAt: run.expiresAt,
	}
	for i, raw := range run.events {
		if first+uint64(i) > after {
			result.Events = append(result.Events, bytes.Clone(raw))
		}
	}
	return result, nil
}

func (s *runStore) clearOwner(owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.scope.owner == owner {
			s.remove(run)
		}
	}
}

func (s *runStore) clearPatient(patient string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.scope.patient == patient {
			s.remove(run)
		}
	}
}

func (s *runStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		s.remove(run)
	}
}

func (s *runStore) release(owner, patient string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runs[s.currentID[runScope{owner, patient}]]; run != nil {
		s.remove(run)
		return true
	}
	return s.locks.Release(patient, owner)
}
