package main

import (
	"slices"
	"time"
)

const sourceSelectionTTL = 5 * time.Minute

// sourceSelection is one discovery result. Its generation is the patient's
// retrieval generation when discovery started, so a recycle that replaced the
// data underneath it invalidates the selection as it invalidates a retrieval.
type sourceSelection struct {
	id         string
	expiresAt  time.Time
	generation uint64
	sources    []localizedSource
}

func (s *runStore) rememberSources(id, owner string, sources []localizedSource, generation uint64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	run := s.runs[id]
	if run == nil || run.scope.owner != owner {
		return "", errRunUnavailable
	}
	selection := &sourceSelection{id: randomURLSafe(), expiresAt: now.Add(sourceSelectionTTL), generation: generation}
	for _, source := range sources {
		source.Categories = slices.Clone(source.Categories)
		selection.sources = append(selection.sources, source)
	}
	run.sources = selection
	return selection.id, nil
}

func (s *runStore) selectedSource(id, owner, selectionID, ura string, generation uint64) (localizedSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	run := s.runs[id]
	if run == nil || run.scope.owner != owner || run.sources == nil {
		return localizedSource{}, false
	}
	if !run.sources.expiresAt.After(now) {
		run.sources = nil
		return localizedSource{}, false
	}
	if selectionID == "" || run.sources.id != selectionID || run.sources.generation != generation {
		return localizedSource{}, false
	}
	for _, source := range run.sources.sources {
		if source.URA == ura && source.Addressable() {
			source.Categories = slices.Clone(source.Categories)
			return source, true
		}
	}
	return localizedSource{}, false
}
