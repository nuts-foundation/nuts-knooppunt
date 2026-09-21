package subscription

import (
	_ "embed"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
)

// TaskTopicCanonical is the canonical URL of the POC's example SubscriptionTopic: a broad,
// long-lived per-partner subscription on COW Coordination Task changes (eOverdracht
// alignment). Placeholder URL — topic ownership/base is an open working-group question.
const TaskTopicCanonical = "http://fhir.nl/SubscriptionTopic/nl-task-notified-pull"

//go:embed topics/nl-task-notified-pull.json
var taskTopicJSON []byte

// taskTopic is the parsed design-time artifact, served read-only at
// GET /fhir/SubscriptionTopic/{id} for discoverability.
var taskTopic = func() fhirsubscription.SubscriptionTopic {
	topic, err := fhirsubscription.UnmarshalSubscriptionTopic(taskTopicJSON)
	if err != nil {
		panic("embedded SubscriptionTopic is invalid: " + err.Error())
	}
	return topic
}()

// event is a normalised application event, fed by the internal ingest API or the Task
// _history poller, before topic matching.
type event struct {
	ResourceType string
	ResourceID   string
	Interaction  string // create | update | delete
	OwnerURA     string
	UseCaseCode  string
}

// matchesTopic evaluates whether ev matches sub's topic and filter criteria.
//
// Deliberately minimal (plan D4): evaluating resourceTrigger.queryCriteria/canFilterBy
// generically in Go against arbitrary resources is exactly the vendor cost the topic
// granularity exploration (doc 04) predicts, so the semantics of the single agreed topic
// are hard-coded — Task resource, create/update interactions, owner-URA filter — and the
// general problem is written up as a finding (research questions 3/4).
func (c *Component) matchesTopic(sub *serverSubscription, ev event) bool {
	if sub.Topic != TaskTopicCanonical {
		return false
	}
	trigger := taskTopic.ResourceTrigger[0]
	if ev.ResourceType != trigger.Resource {
		return false
	}
	interactionSupported := false
	for _, interaction := range trigger.SupportedInteraction {
		if ev.Interaction == interaction {
			interactionSupported = true
			break
		}
	}
	if !interactionSupported {
		return false
	}
	// Owner filter: a backport-filter-criteria of the form Task.owner=<...URA value>.
	// An event without owner information matches any owner filter — the poller/ingest
	// may not know the owner, and silently dropping would be worse for a POC. Logged
	// as permissive matching.
	if ownerFilter := sub.OwnerURAFilter(); ownerFilter != "" && ev.OwnerURA != "" && ownerFilter != ev.OwnerURA {
		return false
	}
	return true
}

// filterURA extracts the URA value from a Task.owner filter criteria string, accepting
// both token form (Task.owner=http://fhir.nl/fhir/NamingSystem/ura|00000020) and a bare
// value (Task.owner=00000020).
func filterURA(criteria string) string {
	value, found := strings.CutPrefix(criteria, "Task.owner=")
	if !found {
		return ""
	}
	if system, code, hasSystem := strings.Cut(value, "|"); hasSystem {
		_ = system
		return code
	}
	// Also tolerate reference form Organization/... — not a URA, ignore.
	if strings.Contains(value, "/") {
		return ""
	}
	return value
}
