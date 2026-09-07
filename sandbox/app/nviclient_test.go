package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

func TestPatientShareStatus_SharedWhenCategoriesExist(t *testing.T) {
	cfg := Config{nviCategories: func(context.Context, string) ([]string, error) {
		return []string{nvi.CategoryCondition, nvi.CategoryPatient}, nil
	}}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.Shared)
	require.False(t, status.NVIUnknown)
	require.Equal(t, []string{nvi.CategoryCondition, nvi.CategoryPatient}, status.Categories)
}

func TestPatientShareStatus_LocalOnlyWhenNoCategories(t *testing.T) {
	cfg := Config{nviCategories: func(context.Context, string) ([]string, error) { return nil, nil }}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.False(t, status.Shared)
	require.False(t, status.NVIUnknown)
}

// A failing NVI must produce an unknown chip, not a failed page and not "local
// only": the latter would invite a presenter to share an already-shared patient.
func TestPatientShareStatus_UnknownWhenTheNVIFails(t *testing.T) {
	cfg := Config{nviCategories: func(context.Context, string) ([]string, error) {
		return nil, errors.New("connection refused")
	}}

	require.True(t, cfg.patientShareStatus(t.Context(), "999900006").NVIUnknown)
}

// http.DefaultClient carries no timeout, so without a deadline a blackholed NVI
// hangs the request goroutine and the page forever.
func TestPatientShareStatus_UnknownWhenTheCallExceedsItsDeadline(t *testing.T) {
	cfg := Config{nviCategories: func(ctx context.Context, _ string) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
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
