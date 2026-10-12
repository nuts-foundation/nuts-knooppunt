package mitzmock

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const anna, bram = "999911120", "999911132"

type consentBody struct {
	BSN           string               `json:"bsn"`
	Decision      string               `json:"decision"`
	Overridden    bool                 `json:"overridden"`
	ChangeID      string               `json:"changeId"`
	ChangedAt     time.Time            `json:"changedAt"`
	Watermark     time.Time            `json:"watermark"`
	Notifications []notificationResult `json:"notifications"`
}

func consentCall(t *testing.T, mitz *ClosedQuestionService, method, path, body string) (int, consentBody) {
	t.Helper()
	req, err := http.NewRequest(method, mitz.GetURL()+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	var parsed consentBody
	if len(bytes.TrimSpace(raw)) > 0 && res.Header.Get("Content-Type") == "application/json" {
		require.NoError(t, json.Unmarshal(raw, &parsed), "body: %s", raw)
	}
	return res.StatusCode, parsed
}

func decisionFor(t *testing.T, mitz *ClosedQuestionService, body string) (int, string) {
	t.Helper()
	res := ask(t, mitz, body, "")
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	switch {
	case strings.Contains(string(raw), "<Decision>Permit</Decision>"):
		return res.StatusCode, "Permit"
	case strings.Contains(string(raw), "<Decision>Deny</Decision>"):
		return res.StatusCode, "Deny"
	}
	return res.StatusCode, ""
}

func TestConsent_AnOverrideAnswersThatPatientOnly(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	status, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, anna, put.BSN)
	require.Equal(t, "Deny", put.Decision)
	require.False(t, put.ChangedAt.IsZero())
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, put.ChangeID, "a change id is generated when none is given")
	require.NotNil(t, put.Notifications, "notifications is an array, also when empty")

	_, annaDecision := decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))
	_, bramDecision := decisionFor(t, mitz, closedQuestion(bram, "00000020", "00000010"))
	require.Equal(t, "Deny", annaDecision)
	require.Equal(t, "Permit", bramDecision, "another patient keeps the default")
}

func TestConsent_GetReportsTheCurrentAnswer(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	status, before := consentCall(t, mitz, http.MethodGet, "/mock/consent/"+anna, "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Permit", before.Decision)
	require.False(t, before.Overridden)
	require.True(t, before.ChangedAt.IsZero())

	_, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	_, after := consentCall(t, mitz, http.MethodGet, "/mock/consent/"+anna, "")
	require.Equal(t, "Deny", after.Decision)
	require.True(t, after.Overridden)
	require.True(t, after.ChangedAt.Equal(put.ChangedAt))

	other := NewClosedQuestionService(t)
	other.SetResponse("Deny", "No consent found")
	_, configured := consentCall(t, other, http.MethodGet, "/mock/consent/"+anna, "")
	require.Equal(t, "Deny", configured.Decision, "the default reported is the configured one")
}

func TestConsent_TheMagicStringsComeBeforeAnOverride(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Permit"}`)
	mitz.SetResponse("Permit", "Consent granted")

	status, decision := decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010")+"<!-- bsn:deny bsn:error -->")
	require.Equal(t, http.StatusOK, status, "bsn:deny is looked for before bsn:error, as it always was")
	require.Equal(t, "Deny", decision)

	status, _ = decisionFor(t, mitz, "bsn:error")
	require.Equal(t, http.StatusInternalServerError, status)
}

func TestConsent_AnUnreadableBodyGetsTheDefault(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	_, decision := decisionFor(t, mitz, "not xml at all")
	require.Equal(t, "Permit", decision)
}

func TestConsent_RefusesAnythingButPermitAndDeny(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	for _, body := range []string{``, `{}`, `{"decision":"permit"}`, `{"decision":"Indeterminate"}`, `not json`,
		`{"decision":"Deny","changeId":"not-a-uuid"}`, `{"decision":"Deny","changeId":"4BF92F35-77B3-4DA6-A3CE-929D0E0E4736"}`} {
		status, _ := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, body)
		require.Equalf(t, http.StatusBadRequest, status, "body %q", body)
	}
	_, current := consentCall(t, mitz, http.MethodGet, "/mock/consent/"+anna, "")
	require.False(t, current.Overridden, "a refused change stores nothing")
}

func TestConsent_ASuppliedChangeIDIsReturned(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	const id = "4bf92f35-77b3-4da6-a3ce-929d0e0e4736"
	_, put := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny","changeId":"`+id+`"}`)
	require.Equal(t, id, put.ChangeID)
}

func TestConsent_DeleteRestoresTheDefaultAsSetResponseSetIt(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	mitz.SetResponse("Deny", "No consent found") // a configured default other than Permit
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Permit"}`)
	_, overridden := decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))
	require.Equal(t, "Permit", overridden)

	status, deleted := consentCall(t, mitz, http.MethodDelete, "/mock/consent/"+anna, "")
	require.Equal(t, http.StatusOK, status)
	require.False(t, deleted.Watermark.IsZero())
	_, restored := decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))
	require.Equal(t, "Deny", restored, "the default is whatever SetResponse configured, not a hardcoded Permit")
}

func TestConsent_DeleteOneLeavesAnotherPatientAndItsLog(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+bram, `{"decision":"Deny"}`)
	decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))
	decisionFor(t, mitz, closedQuestion(bram, "00000020", "00000010"))
	requestsBefore := mitz.RequestCount()

	consentCall(t, mitz, http.MethodDelete, "/mock/consent/"+anna, "")

	_, bramConsent := consentCall(t, mitz, http.MethodGet, "/mock/consent/"+bram, "")
	require.True(t, bramConsent.Overridden)
	log := mitz.Decisions()
	require.Len(t, log, 1)
	require.Equal(t, bram, log[0].PatientID)
	require.Equal(t, requestsBefore, mitz.RequestCount(), "the raw request slice is not part of the consent state")
}

func TestConsent_DeleteAllClearsEveryOverrideAndTheLog(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	consentCall(t, mitz, http.MethodPut, "/mock/consent/"+bram, `{"decision":"Deny"}`)
	decisionFor(t, mitz, closedQuestion(anna, "00000020", "00000010"))

	status, deleted := consentCall(t, mitz, http.MethodDelete, "/mock/consent", "")
	require.Equal(t, http.StatusOK, status)
	require.False(t, deleted.Watermark.IsZero())
	for _, bsn := range []string{anna, bram} {
		_, current := consentCall(t, mitz, http.MethodGet, "/mock/consent/"+bsn, "")
		require.False(t, current.Overridden)
	}
	require.Empty(t, mitz.Decisions())
}

func TestConsent_AWatermarkSitsBetweenTheChangesAroundIt(t *testing.T) {
	mitz := NewClosedQuestionService(t)
	_, before := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	_, one := consentCall(t, mitz, http.MethodDelete, "/mock/consent/"+anna, "")
	_, between := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+anna, `{"decision":"Deny"}`)
	_, all := consentCall(t, mitz, http.MethodDelete, "/mock/consent", "")
	_, after := consentCall(t, mitz, http.MethodPut, "/mock/consent/"+bram, `{"decision":"Deny"}`)

	require.True(t, one.Watermark.After(before.ChangedAt))
	require.True(t, between.ChangedAt.After(one.Watermark))
	require.True(t, all.Watermark.After(between.ChangedAt))
	require.True(t, after.ChangedAt.After(all.Watermark), "the clock is one for the whole mock, not one per patient")
}
