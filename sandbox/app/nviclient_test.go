package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestPatientShareStatus_SharedWhenCategoriesExist(t *testing.T) {
	cfg := Config{nviLookup: func(context.Context, string) (nviRecords, error) {
		return nviRecords{Count: 2, Categories: []string{nvi.CategoryCondition, nvi.CategoryPatient}}, nil
	}}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.Shared)
	require.False(t, status.NVIUnknown)
	require.Equal(t, []string{nvi.CategoryCondition, nvi.CategoryPatient}, status.Categories)
}

func TestPatientShareStatus_LocalOnlyWhenNoCategories(t *testing.T) {
	cfg := Config{nviLookup: func(context.Context, string) (nviRecords, error) { return nviRecords{}, nil }}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.False(t, status.Shared)
	require.False(t, status.NVIUnknown)
}

// A List this build cannot decode is still a List. Deriving "shared" from the
// recognized categories would tell the presenter a findable patient is
// local-only. Reachable on a stack whose NVI still holds records from an earlier
// seed: compose reuses the hapi-fhir container, and the client-scoped delete
// only removes what this build wrote.
func TestPatientShareStatus_SharedWhenAListHasNoRecognizedCategory(t *testing.T) {
	cfg := Config{nviLookup: func(context.Context, string) (nviRecords, error) {
		return nviRecords{Count: 1, Unnamed: 1}, nil
	}}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.Shared, "a List the vocabulary does not recognize still makes the patient findable")
	require.Empty(t, status.Categories)
	require.False(t, status.NVIUnknown)
}

// A failing NVI must produce an unknown chip, not a failed page and not "local
// only": the latter would invite a presenter to share an already-shared patient.
func TestPatientShareStatus_UnknownWhenTheNVIFails(t *testing.T) {
	cfg := Config{nviLookup: func(context.Context, string) (nviRecords, error) {
		return nviRecords{}, errors.New("connection refused")
	}}

	require.True(t, cfg.patientShareStatus(t.Context(), "999900006").NVIUnknown)
}

// http.DefaultClient carries no timeout, so without a deadline a blackholed NVI
// hangs the request goroutine and the page forever.
func TestPatientShareStatus_UnknownWhenTheCallExceedsItsDeadline(t *testing.T) {
	cfg := Config{nviLookup: func(ctx context.Context, _ string) (nviRecords, error) {
		<-ctx.Done()
		return nviRecords{}, ctx.Err()
	}}

	start := time.Now()
	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.NVIUnknown)
	require.Less(t, time.Since(start), nviCallTimeout*2)
}

func TestPatientShareStatus_UnknownWhenUnconfigured(t *testing.T) {
	require.True(t, Config{}.patientShareStatus(t.Context(), "999900006").NVIUnknown)
}

// The share must send exactly the categories De Plataan holds, derived from the
// patient's resources rather than from a constant.
func TestRegisterPatient_SendsTheDerivedCategories(t *testing.T) {
	var gotBSN string
	var gotCategories []string
	cfg := Config{nviRegister: func(_ context.Context, bsn string, cats []string) error {
		gotBSN, gotCategories = bsn, cats
		return nil
	}}
	anna := pool.Patients()[0]

	require.NoError(t, cfg.registerPatient(t.Context(), anna))

	require.Equal(t, anna.BSN, gotBSN)
	require.Equal(t, anna.PlataanCategories(), gotCategories)
}

func TestRegisterPatient_ErrorsWhenUnconfigured(t *testing.T) {
	require.Error(t, Config{}.registerPatient(t.Context(), pool.Patients()[0]))
}

// envOf returns a getenv over a fixed set of variables, everything else unset.
func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// The NVI client talks only to the Knooppunt, so it must come up on
// KNOOPPUNT_INTERNAL_URL alone. Gating it on HAPI_BASE_URL as well left every
// patient row reading "status unknown" with nothing to say why.
func TestNewConfigFromEnv_WiresTheNVIWithoutHAPI(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{"KNOOPPUNT_INTERNAL_URL": "http://knooppunt:8081"}))

	require.NoError(t, err)
	require.True(t, cfg.nviConfigured(), "the NVI needs only the Knooppunt URL")
	require.False(t, cfg.configured(), "reset and recycle still need HAPI_BASE_URL")
}

func TestNewConfigFromEnv_WiresResetWhenBothAreSet(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{
		"KNOOPPUNT_INTERNAL_URL": "http://knooppunt:8081",
		"HAPI_BASE_URL":          "http://hapi:7050/fhir",
	}))

	require.NoError(t, err)
	require.True(t, cfg.nviConfigured())
	require.True(t, cfg.configured())
}

func TestNewConfigFromEnv_NothingIsWiredWithoutTheKnooppunt(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(nil))

	require.NoError(t, err)
	require.False(t, cfg.nviConfigured())
	require.False(t, cfg.configured())
}

// SANDBOX_NVI_CLIENT_ID has to reach publication and cleanup as one value: the
// client id is the scope a registration is deleted by, so an override that moved
// only the publishing half would leave recycle deleting under an id nothing was
// written with, while its notice still said the patient was restored. Observed at
// the wire, because both paths are closures and asserting that they were assigned
// says nothing about which id they carry.
func TestNewConfigFromEnv_TheOverriddenClientIDReachesPublicationAndCleanup(t *testing.T) {
	const override = "gf-sandbox-plataan-override"
	var mu sync.Mutex
	var seen []string
	nviFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			if source := r.Form.Get("source:identifier"); source != "" {
				mu.Lock()
				seen = append(seen, source)
				mu.Unlock()
			}
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset"}`))
	}))
	defer nviFake.Close()

	cfg, err := NewConfigFromEnv(envOf(map[string]string{
		"KNOOPPUNT_INTERNAL_URL": nviFake.URL,
		"SANDBOX_NVI_CLIENT_ID":  override,
		// Unreachable on purpose: recycle deletes De Plataan's NVI records before
		// it touches HAPI, so the client id is already on the wire by the time
		// this fails, and nothing here needs a FHIR store.
		"HAPI_BASE_URL": "http://127.0.0.1:1/fhir",
	}))
	require.NoError(t, err)

	anna := pool.Patients()[0]
	require.NoError(t, cfg.nviRegister(t.Context(), anna.BSN, anna.PlataanCategories()))
	_ = cfg.recyclePatient(t.Context(), anna.Key)

	mu.Lock()
	defer mu.Unlock()
	overridden := 0
	for _, source := range seen {
		if source == nvi.OAuthClientIDSystem+"|"+override {
			overridden++
		}
	}
	require.Equal(t, 2, overridden,
		"publication and De Plataan's cleanup must both scope by the configured client id")
	require.Contains(t, seen, nvi.OAuthClientIDSystem+"|"+pool.ZonnebloemClientID,
		"and the override must not follow along into the source side's own registration")
}

// The adapter is where the unnamed count is produced, and every other test in
// this file injects an already-populated nviRecords, so none of them can see it
// stop being produced: a constant zero there would cost a mixed real response its
// warning while the helper's own test stays green.
func TestNVIFuncs_CarriesTheUnnamedCountFromTheRealResponse(t *testing.T) {
	reg := nvi.Registration{CustodianURA: plataan.URA, BSN: "999900006", ClientID: "c"}
	recognized := nvi.BuildList(reg, nvi.CategoryCondition)
	foreign := nvi.BuildList(reg, nvi.CategoryCondition)
	foreign.Code.Coding[0].System = to.Ptr("http://example.test/legacy-categories")

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entries := make([]fhir.BundleEntry, 0, 2)
		for _, list := range []fhir.List{recognized, foreign} {
			raw, err := json.Marshal(list)
			require.NoError(t, err)
			entries = append(entries, fhir.BundleEntry{Resource: raw})
		}
		body, err := json.Marshal(fhir.Bundle{Type: fhir.BundleTypeSearchset, Entry: entries})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write(body)
	}))
	defer fake.Close()
	base, err := url.Parse(fake.URL)
	require.NoError(t, err)

	lookup, _ := nviFuncs(base, "c")
	records, err := lookup(t.Context(), reg.BSN)

	require.NoError(t, err)
	require.Equal(t, 2, records.Count)
	require.Equal(t, []string{nvi.CategoryCondition}, records.Categories)
	require.Equal(t, 1, records.Unnamed,
		"the adapter must report the record this build cannot name, not derive it from the category set")
}
