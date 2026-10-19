package mitzmock

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// maxDecisions bounds the decision log to its most recent entries.
const maxDecisions = 512

// Decision is one closed question the mock answered. The log exists for the GF
// Sandbox's viewer, which shows Mitz's answer to a source request; the national
// Mitz has nothing like it.
type Decision struct {
	Seq           uint64    `json:"seq"`
	Time          time.Time `json:"time"`
	DurationMs    int64     `json:"durationMs"`
	TraceID       string    `json:"traceId"`
	PatientID     string    `json:"patientId"`
	DataHolderURA string    `json:"dataHolderUra"`
	RequesterURA  string    `json:"requesterUra"`
	Decision      string    `json:"decision"`
	Status        int       `json:"status"`
	Request       string    `json:"request"`
	Response      string    `json:"response"`
}

type decisionLog struct {
	mu      sync.Mutex
	seq     uint64
	entries []Decision
}

func (l *decisionLog) add(d Decision) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	d.Seq = l.seq
	l.entries = append(l.entries, d)
	if len(l.entries) > maxDecisions {
		l.entries = append([]Decision(nil), l.entries[len(l.entries)-maxDecisions:]...)
	}
}

func (l *decisionLog) all() []Decision {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Decision(nil), l.entries...)
}

func (l *decisionLog) byTrace(traceID string) []Decision {
	l.mu.Lock()
	defer l.mu.Unlock()
	found := []Decision{}
	for _, entry := range l.entries {
		if entry.TraceID == traceID {
			found = append(found, entry)
		}
	}
	return found
}

// deletePatient drops one patient's entries. The sequence counter is left
// alone: a sequence is never reused.
func (l *decisionLog) deletePatient(bsn string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.entries[:0]
	for _, entry := range l.entries {
		if entry.PatientID != bsn {
			kept = append(kept, entry)
		}
	}
	l.entries = kept
}

func (l *decisionLog) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// traceparent is the W3C Trace Context header, version 00.
var traceparent = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)

// traceIDFrom returns the trace id of a traceparent header, or "" when the
// header is absent or malformed, or carries an all-zero trace id or parent id.
// The Knooppunt's propagator (go.opentelemetry.io/otel/propagation) drops a
// header with either, so no closed question it sends can carry one.
func traceIDFrom(header string) string {
	match := traceparent.FindStringSubmatch(header)
	if match == nil || match[1] == strings.Repeat("0", 32) || match[2] == strings.Repeat("0", 16) {
		return ""
	}
	return match[1]
}

type closedQuestionSubject struct {
	PatientID     string
	DataHolderURA string
	RequesterURA  string
}

const (
	bsnRoot             = "2.16.840.1.113883.2.4.6.3"
	patientAttribute    = "urn:oasis:names:tc:xacml:2.0:resource:resource-id"
	dataHolderAttribute = "urn:ihe:iti:appc:2016:author-institution:id"
	requesterAttribute  = "urn:nl:otv:names:tc:1.0:subject:provider-institution"
)

// The two elements the mock reads, each by namespace and name. The decoder
// resolves prefixes, so a sender is free in how it binds them.
var (
	attributeElement          = xml.Name{Space: "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17", Local: "Attribute"}
	instanceIdentifierElement = xml.Name{Space: "urn:hl7-org:v3", Local: "InstanceIdentifier"}
)

// parseClosedQuestion reads who a closed question is about from its XACML body:
// the patient from the resource id, the data holder and the requester from
// their attributes. It is this mock's own reading of the request, deliberately
// not the component's: sharing that code would make the two agree by
// construction. Anything it cannot read is left empty.
func parseClosedQuestion(body []byte) closedQuestionSubject {
	var subject closedQuestionSubject
	decoder := xml.NewDecoder(bytes.NewReader(body))
	attribute := ""
	for {
		token, err := decoder.Token()
		if err != nil {
			return subject
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name {
			case attributeElement:
				attribute = attr(element, "AttributeId")
			case instanceIdentifierElement:
				extension := attr(element, "extension")
				switch attribute {
				case patientAttribute:
					if attr(element, "root") == bsnRoot {
						subject.PatientID = extension
					}
				case dataHolderAttribute:
					subject.DataHolderURA = extension
				case requesterAttribute:
					subject.RequesterURA = extension
				}
			}
		case xml.EndElement:
			if element.Name == attributeElement {
				attribute = ""
			}
		}
	}
}

// attr returns the value of the unqualified attribute name.
func attr(element xml.StartElement, name string) string {
	for _, a := range element.Attr {
		if a.Name == (xml.Name{Local: name}) {
			return a.Value
		}
	}
	return ""
}

// Decisions returns every retained decision, oldest first.
func (m *ClosedQuestionService) Decisions() []Decision { return m.decisions.all() }

func (m *ClosedQuestionService) handleDecisions(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("traceid")
	if traceIDFrom("00-"+traceID+"-0000000000000001-00") == "" {
		http.Error(w, "traceid must be 32 lowercase hex digits", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"decisions": m.decisions.byTrace(traceID)})
}
