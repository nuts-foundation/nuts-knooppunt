package subscription

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Event detection by polling (plan D4): the server watches the organisation's FHIR store
// for Task changes via Task/_history?_since — the same idiom the mCSD/LRZA sync uses —
// and feeds matches into the same event pipeline as the internal ingest endpoint. This
// is vendor-integration option (c) from doc 09 (workflow data hosted in a
// knooppunt-adjacent FHIR store); options (a) ingest API and (b) pollable vendor
// endpoint are the other rows of the same finding.

// pollTasks runs one poll cycle: fetch Task history since the last cycle, deduplicate to
// each Task's latest version, and ingest an event per changed Task.
func (c *Component) pollTasks(ctx context.Context) {
	baseURL, err := url.Parse(c.config.Server.FHIRBaseURL)
	if err != nil {
		slog.ErrorContext(ctx, "Invalid subscription server FHIR base URL", logging.Error(err))
		return
	}
	client := fhirclient.New(baseURL, c.httpClient, fhirutil.ClientConfig())

	params := url.Values{}
	c.pollMu.Lock()
	since := c.lastPoll
	c.pollMu.Unlock()
	if since != "" {
		params.Set("_since", since)
	}

	var history fhir.Bundle
	if err := client.SearchWithContext(ctx, "", params, &history, fhirclient.AtPath("Task/_history")); err != nil {
		slog.WarnContext(ctx, "Task history poll failed", logging.Error(err))
		return
	}

	// Record the new watermark before processing: prefer the server's own timestamp,
	// falling back to local time minus a clock-skew buffer (mcsd pattern).
	newWatermark := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	if history.Timestamp != nil {
		newWatermark = *history.Timestamp
	}
	c.pollMu.Lock()
	c.lastPoll = newWatermark
	c.pollMu.Unlock()

	entries := fhirutil.DeduplicateHistoryEntries(history.Entry)
	for _, entry := range entries {
		if entry.Resource == nil {
			continue // DELETE entries: topic covers create/update only
		}
		var task fhir.Task
		if err := json.Unmarshal(entry.Resource, &task); err != nil || task.Id == nil {
			continue
		}
		interaction := "update"
		if task.Meta != nil && task.Meta.VersionId != nil && *task.Meta.VersionId == "1" {
			interaction = "create"
		}
		ev := event{
			ResourceType: "Task",
			ResourceID:   *task.Id,
			Interaction:  interaction,
			OwnerURA:     taskOwnerURA(task),
			UseCaseCode:  taskUseCaseCode(task),
		}
		matches := c.IngestEvent(ctx, ev, false)
		slog.InfoContext(ctx, "Poller ingested Task change",
			slog.String("task", *task.Id),
			slog.String("interaction", interaction),
			slog.String("ownerUra", ev.OwnerURA),
			slog.Int("matchedSubscriptions", len(matches)))
	}
}

// taskOwnerURA extracts the URA identifier from Task.owner.
func taskOwnerURA(task fhir.Task) string {
	if task.Owner == nil || task.Owner.Identifier == nil {
		return ""
	}
	identifier := task.Owner.Identifier
	if identifier.System != nil && *identifier.System == coding.URANamingSystem && identifier.Value != nil {
		return *identifier.Value
	}
	return ""
}

// taskUseCaseCode extracts the non-task-code coding from Task.code (the COW profile's
// second coding, e.g. the eOverdracht use-case code).
func taskUseCaseCode(task fhir.Task) string {
	if task.Code == nil {
		return ""
	}
	for _, c := range task.Code.Coding {
		if c.System != nil && *c.System == "http://hl7.org/fhir/CodeSystem/task-code" {
			continue
		}
		if c.Code != nil {
			return *c.Code
		}
	}
	return ""
}
