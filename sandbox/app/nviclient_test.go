package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
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
// recognized categories would tell the presenter the patient is local-only while
// other providers can find them, which is the demo denying a registration that
// exists. Reachable on a stack whose NVI still holds records from an earlier
// seed: compose reuses an unchanged hapi-fhir container, and the client-scoped
// delete only removes what this build wrote.
func TestPatientShareStatus_SharedWhenAListHasNoRecognizedCategory(t *testing.T) {
	cfg := Config{nviLookup: func(context.Context, string) (nviRecords, error) {
		return nviRecords{Count: 1}, nil
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

// The NVI client talks only to the Knooppunt, so it must come up on
// KNOOPPUNT_INTERNAL_URL alone. Gating it on HAPI_BASE_URL as well disabled it
// over a variable it never reads, and the only symptom was every patient row
// reading "status unknown" with nothing to say why.
func TestNewConfigFromEnv_WiresTheNVIWithoutHAPI(t *testing.T) {
	cfg, err := NewConfigFromEnv(func(key string) string {
		if key == "KNOOPPUNT_INTERNAL_URL" {
			return "http://knooppunt:8081"
		}
		return ""
	})

	require.NoError(t, err)
	require.True(t, cfg.nviConfigured(), "the NVI needs only the Knooppunt URL")
	require.False(t, cfg.configured(), "reset and recycle still need HAPI_BASE_URL")
}

func TestNewConfigFromEnv_WiresResetWhenBothAreSet(t *testing.T) {
	cfg, err := NewConfigFromEnv(func(key string) string {
		switch key {
		case "KNOOPPUNT_INTERNAL_URL":
			return "http://knooppunt:8081"
		case "HAPI_BASE_URL":
			return "http://hapi:7050/fhir"
		}
		return ""
	})

	require.NoError(t, err)
	require.True(t, cfg.nviConfigured())
	require.True(t, cfg.configured())
}

func TestNewConfigFromEnv_NothingIsWiredWithoutTheKnooppunt(t *testing.T) {
	cfg, err := NewConfigFromEnv(func(string) string { return "" })

	require.NoError(t, err)
	require.False(t, cfg.nviConfigured())
	require.False(t, cfg.configured())
}

// SANDBOX_NVI_CLIENT_ID has to reach publication and cleanup as one value. The
// client id is the scope a registration is deleted by, so an override that moves
// only the publishing half leaves recycle deleting under an id nothing was
// written with, while its notice still says the patient was restored.
//
// Observed at the wire, not at the wiring: both paths are closures, and asserting
// that they were assigned says nothing about which id they carry.
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

	cfg, err := NewConfigFromEnv(func(key string) string {
		switch key {
		case "KNOOPPUNT_INTERNAL_URL":
			return nviFake.URL
		case "HAPI_BASE_URL":
			// Unreachable on purpose: recycle deletes De Plataan's NVI records
			// before it touches HAPI, so the client id is already on the wire by
			// the time this fails, and nothing here needs a FHIR store.
			return "http://127.0.0.1:1/fhir"
		case "SANDBOX_NVI_CLIENT_ID":
			return override
		}
		return ""
	})
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
