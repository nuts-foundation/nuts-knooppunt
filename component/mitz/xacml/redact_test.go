package xacml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactBSN_RequestXML(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "900186021",
		HealthcareFacilityType: "Z3",
		AuthorInstitutionID:    "00000659",
		EventCode:              "GGC002",
		SubjectRole:            "01.015",
		ProviderID:             "000095254",
		ProviderInstitutionID:  "00000666",
		ConsultingFacilityType: "Z3",
		PurposeOfUse:           "TREAT",
	}
	xml, err := CreateAuthzDecisionQuery(req)
	require.NoError(t, err)

	// Serializer-shape guard: RedactBSN's regexes assume the current etree
	// serialization (double-quoted attributes, no spaces around `=`, bare
	// local-name attributes). If a future change to the serializer breaks any
	// of these assumptions this assertion fails first, naming the cause
	// rather than having the redaction assertion fail ambiguously.
	require.Contains(t, xml, `extension="900186021"`, "serializer shape drift: expected double-quoted attribute without spaces")
	require.Contains(t, xml, `root="`+BSNRootOID+`"`, "serializer shape drift: expected bare root attribute with BSN OID")

	redacted := RedactBSN(xml)

	assert.NotContains(t, redacted, "900186021", "BSN must not appear in redacted output")
	assert.Contains(t, redacted, "[REDACTED]")
	// Non-BSN identifiers must be preserved so the log remains useful for debugging.
	assert.Contains(t, redacted, "00000659", "AuthorInstitutionID must be preserved")
	assert.Contains(t, redacted, "000095254", "ProviderID must be preserved")
	assert.Contains(t, redacted, "00000666", "ProviderInstitutionID must be preserved")
	assert.Contains(t, redacted, "XACMLAuthzDecisionQuery")
}

// The signed request path adds an XML-DSig Signature element to the XACML
// query. Redaction must keep working on that serialized output.
func TestRedactBSN_SignedRequestXML(t *testing.T) {
	signingConfig := generateTestSigningConfig(t)
	req := AuthzRequest{
		PatientBSN:             "900186021",
		HealthcareFacilityType: "Z3",
		AuthorInstitutionID:    "00000659",
		EventCode:              "GGC002",
		SubjectRole:            "01.015",
		ProviderID:             "000095254",
		ProviderInstitutionID:  "00000666",
		ConsultingFacilityType: "Z3",
		PurposeOfUse:           "TREAT",
	}
	xml, err := CreateSignedAuthzDecisionQuery(req, signingConfig)
	require.NoError(t, err)
	require.Contains(t, xml, "900186021")
	require.Contains(t, xml, "ds:SignedInfo", "sanity: signed path must produce a Signature element")

	redacted := RedactBSN(xml)

	assert.NotContains(t, redacted, "900186021", "BSN must not appear in redacted signed payload")
	assert.Contains(t, redacted, "[REDACTED]")
	assert.Contains(t, redacted, "ds:SignedInfo", "signature structure must be preserved")
	assert.Contains(t, redacted, "00000659", "non-BSN identifiers must be preserved")
}

func TestRedactBSN_ForeignBSNInResponse(t *testing.T) {
	// MITZ echoes the resource-id (BSN) attribute back when IncludeInResult=true,
	// and may include BSNs we did not send ourselves (e.g. legal representative).
	// Redaction is keyed on the root OID, not the BSN value, so foreign BSNs are
	// also scrubbed.
	responseXML := `<Response><InstanceIdentifier extension="999888777" root="2.16.840.1.113883.2.4.6.3" xmlns="urn:hl7-org:v3"/></Response>`
	redacted := RedactBSN(responseXML)
	assert.NotContains(t, redacted, "999888777")
	assert.Contains(t, redacted, `extension="[REDACTED]"`)
}

func TestRedactBSN_LeavesOtherOIDsAlone(t *testing.T) {
	xml := `<Root><InstanceIdentifier extension="12345" root="2.16.528.1.1007.3.3" xmlns="urn:hl7-org:v3"/></Root>`
	redacted := RedactBSN(xml)
	assert.Contains(t, redacted, `extension="12345"`, "non-BSN OIDs must not be redacted")
}

func TestRedactBSN_Empty(t *testing.T) {
	assert.Equal(t, "", RedactBSN(""))
}

func TestRedactBSN_XMLWithoutBSNIsUntouched(t *testing.T) {
	input := `<Envelope><Body>unrelated content</Body></Envelope>`
	assert.Equal(t, input, RedactBSN(input))
}

func TestRedactBSN_RootBeforeExtensionOrdering(t *testing.T) {
	xml := `<Root><InstanceIdentifier root="2.16.840.1.113883.2.4.6.3" extension="900186021" xmlns="urn:hl7-org:v3"/></Root>`
	redacted := RedactBSN(xml)
	assert.NotContains(t, redacted, "900186021")
	assert.Contains(t, redacted, `extension="[REDACTED]"`)
}

func TestRedactBSN_MultipleBSNElements(t *testing.T) {
	xml := `<Root>` +
		`<InstanceIdentifier extension="900186021" root="2.16.840.1.113883.2.4.6.3" xmlns="urn:hl7-org:v3"/>` +
		`<InstanceIdentifier extension="900186022" root="2.16.840.1.113883.2.4.6.3" xmlns="urn:hl7-org:v3"/>` +
		`</Root>`
	redacted := RedactBSN(xml)
	assert.NotContains(t, redacted, "900186021")
	assert.NotContains(t, redacted, "900186022")
	assert.Equal(t, 2, strings.Count(redacted, `extension="[REDACTED]"`))
}
