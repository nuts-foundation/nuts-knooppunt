package main

import (
	"slices"
	"time"
)

const sourceSelectionTTL = 5 * time.Minute

type sourceSelection struct {
	id        string
	expiresAt time.Time
	sources   []localizedSource
}

func (s *runStore) rememberSources(id, owner string, sources []localizedSource) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.prune(now)
	run := s.runs[id]
	if run == nil || run.scope.owner != owner {
		return "", errRunUnavailable
	}
	selection := &sourceSelection{id: randomURLSafe(), expiresAt: now.Add(sourceSelectionTTL)}
	for _, source := range sources {
		source.Categories = slices.Clone(source.Categories)
		selection.sources = append(selection.sources, source)
	}
	run.sources = selection
	return selection.id, nil
}

func (s *runStore) selectedSource(id, owner, selectionID, ura string) (localizedSource, bool) {
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
	if selectionID == "" || run.sources.id != selectionID {
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
