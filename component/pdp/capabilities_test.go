package pdp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponent_enrichPolicyInputWithCapabilityStatement(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		inp, reasons := enrichPolicyInputWithCapabilityStatement(context.Background(), PolicyInput{}, "mcsd_update")
		assert.Empty(t, reasons)
		assert.NotNil(t, inp.CapabilityStatement)
	})
	t.Run("not found", func(t *testing.T) {
		inp, reasons := enrichPolicyInputWithCapabilityStatement(context.Background(), PolicyInput{}, "other")
		assert.NotEmpty(t, reasons)
		assert.Nil(t, inp.CapabilityStatement)
		assert.Equal(t, TypeResultCodeUnexpectedInput, reasons[0].Code)
		assert.Contains(t, reasons[0].Description, "missing FHIR CapabilityStatement 'other'")
	})
}
