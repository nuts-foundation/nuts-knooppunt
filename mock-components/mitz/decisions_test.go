package mitzmock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// closedQuestion is a request shaped like the Knooppunt's, written out here
// rather than built with the component's own types: a mock that reads requests
// with the code that writes them can only agree with itself.
func closedQuestion(bsn, dataHolder, requester string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope" xmlns:types="urn:oasis:names:tc:xacml:3.0:profile:saml2.0:v2:schema:protocol:wd-14">
  <SOAP-ENV:Body>
    <types:XACMLAuthzDecisionQuery>
      <Request xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
        <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
          <Attribute AttributeId="urn:oasis:names:tc:xacml:2.0:resource:resource-id" IncludeInResult="true">
            <AttributeValue DataType="urn:hl7-org:v3#II"><InstanceIdentifier extension="%s" root="2.16.840.1.113883.2.4.6.3" xmlns="urn:hl7-org:v3"/></AttributeValue>
          </Attribute>
          <Attribute AttributeId="urn:ihe:iti:appc:2016:author-institution:id" IncludeInResult="true">
            <AttributeValue DataType="urn:hl7-org:v3#II"><InstanceIdentifier extension="%s" root="2.16.528.1.1007.3.3" xmlns="urn:hl7-org:v3"/></AttributeValue>
          </Attribute>
        </Attributes>
        <Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">
          <Attribute AttributeId="urn:ihe:iti:xua:2017:subject:provider-identifier" IncludeInResult="true">
            <AttributeValue DataType="urn:hl7-org:v3#II"><InstanceIdentifier extension="900001234" root="2.16.528.1.1007.3.1" xmlns="urn:hl7-org:v3"/></AttributeValue>
          </Attribute>
          <Attribute AttributeId="urn:nl:otv:names:tc:1.0:subject:provider-institution" IncludeInResult="true">
            <AttributeValue DataType="urn:hl7-org:v3#II"><InstanceIdentifier extension="%s" root="2.16.528.1.1007.3.3" xmlns="urn:hl7-org:v3"/></AttributeValue>
          </Attribute>
        </Attributes>
      </Request>
    </types:XACMLAuthzDecisionQuery>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`, bsn, dataHolder, requester)
}

func ask(t *testing.T, mitz *ClosedQuestionService, body, traceparent string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, mitz.GetURL()+"/geslotenautorisatievraag/xacml3", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/xml")
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { res.Body.Close() })
	return res
}

const traceA = "4bf92f3577b34da6a3ce929d0e0e4736"

func TestParseClosedQuestion_ReadsPatientHolderAndRequester(t *testing.T) {
	q := parseClosedQuestion([]byte(closedQuestion("999911120", "00000020", "00000010")))
	require.Equal(t, closedQuestionSubject{PatientID: "999911120", DataHolderURA: "00000020", RequesterURA: "00000010"}, q)
}

func TestParseClosedQuestion_UnreadableBodyYieldsNothing(t *testing.T) {
	require.Equal(t, closedQuestionSubject{}, parseClosedQuestion([]byte("bsn:deny")))
}

// The request the Knooppunt documents as its example, read from the file and
// not built by the component's code.
func TestParseClosedQuestion_ReadsTheComponentsExampleRequest(t *testing.T) {
	example, err := os.ReadFile("../../component/mitz/xacml/example/example.xml")
	require.NoError(t, err)
	require.Equal(t, closedQuestionSubject{PatientID: "900186021", DataHolderURA: "00000659", RequesterURA: "00000666"},
		parseClosedQuestion(example))
}

// An element is recognised by its namespace and its name, so the same names in
// another vocabulary are not a closed question, and nothing is read from them.
func TestParseClosedQuestion_ReadsNothingFromElementsInAnotherNamespace(t *testing.T) {
	question := closedQuestion("999911120", "00000020", "00000010")
	for name, body := range map[string]string{
		"identifiers outside HL7 v3":   strings.ReplaceAll(question, `xmlns="urn:hl7-org:v3"`, `xmlns="urn:example:not-hl7"`),
		"attributes outside XACML 3.0": strings.ReplaceAll(question, `xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"`, `xmlns="urn:example:not-xacml"`),
	} {
		require.NotEqual(t, question, body, "the fixture carries the namespace this case replaces")
		require.Equalf(t, closedQuestionSubject{}, parseClosedQuestion([]byte(body)), "%s", name)
	}
}

// The namespaces decide, not the prefixes a sender happens to bind them to.
func TestParseClosedQuestion_DoesNotDependOnPrefixes(t *testing.T) {
	const prefixed = `<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:x="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17" xmlns:h="urn:hl7-org:v3"><e:Body><x:Request>
<x:Attributes><x:Attribute AttributeId="urn:oasis:names:tc:xacml:2.0:resource:resource-id"><x:AttributeValue><h:InstanceIdentifier extension="999911120" root="2.16.840.1.113883.2.4.6.3"/></x:AttributeValue></x:Attribute>
<x:Attribute AttributeId="urn:ihe:iti:appc:2016:author-institution:id"><x:AttributeValue><h:InstanceIdentifier extension="00000020" root="2.16.528.1.1007.3.3"/></x:AttributeValue></x:Attribute></x:Attributes>
<x:Attributes><x:Attribute AttributeId="urn:nl:otv:names:tc:1.0:subject:provider-institution"><x:AttributeValue><h:InstanceIdentifier extension="00000010" root="2.16.528.1.1007.3.3"/></x:AttributeValue></x:Attribute></x:Attributes>
</x:Request></e:Body></e:Envelope>`
	require.Equal(t, closedQuestionSubject{PatientID: "999911120", DataHolderURA: "00000020", RequesterURA: "00000010"},
		parseClosedQuestion([]byte(prefixed)))
}

// A BSN is the patient only as the resource id: the same root under another
// attribute names somebody else.
func TestParseClosedQuestion_TakesThePatientOnlyFromTheResourceID(t *testing.T) {
	question := strings.Replace(closedQuestion("999911120", "00000020", "00000010"),
		`AttributeId="urn:oasis:names:tc:xacml:2.0:resource:resource-id"`, `AttributeId="urn:example:somebody-else"`, 1)
	require.Equal(t, closedQuestionSubject{DataHolderURA: "00000020", RequesterURA: "00000010"}, parseClosedQuestion([]byte(question)))
}

func TestTraceIDFrom(t *testing.T) {
	require.Equal(t, traceA, traceIDFrom("00-"+traceA+"-00f067aa0ba902b7-01"))
	for _, malformed := range []string{"", "nonsense", "00-" + strings.ToUpper(traceA) + "-00f067aa0ba902b7-01",
		"00-" + traceA[:31] + "-00f067aa0ba902b7-01", "00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-" + traceA + "-0000000000000000-01"} {
		require.Emptyf(t, traceIDFrom(malformed), "%q is not a usable traceparent", malformed)
	}
}

func TestDecisions_RecordsEachClosedQuestionUnderItsTraceID(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	ask(t, mitz, closedQuestion("999911120", "00000020", "00000010"), "00-"+traceA+"-00f067aa0ba902b7-01")

	res, err := http.Get(mitz.GetURL() + "/mock/decisions?traceid=" + traceA)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body struct {
		Decisions []Decision `json:"decisions"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Decisions, 1)
	got := body.Decisions[0]
	require.Equal(t, uint64(1), got.Seq)
	require.Equal(t, traceA, got.TraceID)
	require.Equal(t, "999911120", got.PatientID)
	require.Equal(t, "00000020", got.DataHolderURA)
	require.Equal(t, "00000010", got.RequesterURA)
	require.Equal(t, "Permit", got.Decision)
	require.Equal(t, http.StatusOK, got.Status)
	require.Contains(t, got.Request, `extension="999911120"`, "the request is kept as received")
	require.Contains(t, got.Response, "<Decision>Permit</Decision>", "the response is kept as answered")
	require.False(t, got.Time.IsZero())
}

func TestDecisions_AnEntryWithoutATraceIDIsNeverReturned(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	ask(t, mitz, closedQuestion("999911120", "00000020", "00000010"), "")

	require.Len(t, mitz.Decisions(), 1, "the question is logged")
	require.Empty(t, mitz.Decisions()[0].TraceID)
	res, err := http.Get(mitz.GetURL() + "/mock/decisions?traceid=" + traceA)
	require.NoError(t, err)
	defer res.Body.Close()
	var body struct {
		Decisions []Decision `json:"decisions"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Empty(t, body.Decisions)
}

func TestDecisions_RefusesAMissingOrMalformedTraceID(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	for _, query := range []string{"", "?traceid=", "?traceid=xyz", "?traceid=" + strings.ToUpper(traceA)} {
		res, err := http.Get(mitz.GetURL() + "/mock/decisions" + query)
		require.NoError(t, err)
		res.Body.Close()
		require.Equalf(t, http.StatusBadRequest, res.StatusCode, "query %q", query)
	}
}

func TestDecisions_TheErrorPathIsLoggedWithItsStatusAndNoDecision(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	res := ask(t, mitz, "bsn:error", "00-"+traceA+"-00f067aa0ba902b7-01")
	require.Equal(t, http.StatusInternalServerError, res.StatusCode)

	entries := mitz.Decisions()
	require.Len(t, entries, 1)
	require.Equal(t, http.StatusInternalServerError, entries[0].Status)
	require.Empty(t, entries[0].Decision)
}

func TestDecisions_KeepsTheMostRecentEntriesAndNeverReusesASequence(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	for i := 0; i < maxDecisions+3; i++ {
		ask(t, mitz, closedQuestion("999911120", "00000020", "00000010"), "")
	}
	entries := mitz.Decisions()
	require.Len(t, entries, maxDecisions)
	require.Equal(t, uint64(4), entries[0].Seq)
	require.Equal(t, uint64(maxDecisions+3), entries[len(entries)-1].Seq)
}
