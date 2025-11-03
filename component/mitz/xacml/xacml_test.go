package xacml

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAuthzDecisionQuery(t *testing.T) {
	// Create request with values from example.xml
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
	require.NotEmpty(t, xml)

	// Verify key elements are present
	assert.Contains(t, xml, "SOAP-ENV:Envelope")
	assert.Contains(t, xml, "XACMLAuthzDecisionQuery")
	assert.Contains(t, xml, "wsa:Action")
	assert.Contains(t, xml, "XACMLAuthorizationDecisionQueryRequest")

	// Verify resource attributes
	assert.Contains(t, xml, "900186021")                 // Patient BSN
	assert.Contains(t, xml, "2.16.840.1.113883.2.4.6.3") // BSN root
	assert.Contains(t, xml, "00000659")                  // Author institution
	assert.Contains(t, xml, "Z3")                        // Healthcare facility type

	// Verify action attributes
	assert.Contains(t, xml, "GGC002") // Event code

	// Verify subject attributes
	assert.Contains(t, xml, "01.015")    // Subject role
	assert.Contains(t, xml, "000095254") // Provider ID
	assert.Contains(t, xml, "CIBG")      // Assigning authority
	assert.Contains(t, xml, "00000666")  // Provider institution

	// Verify environment attributes
	assert.Contains(t, xml, "TREAT") // Purpose of use

	// Verify header elements
	assert.Contains(t, xml, "http://localhost:8000/4")                        // To address
	assert.Contains(t, xml, "urn:uuid:")                                      // Message ID
	assert.Contains(t, xml, "http://www.w3.org/2005/08/addressing/anonymous") // Reply to

	// Verify namespace declarations
	assert.Contains(t, xml, "xmlns:SOAP-ENV=\"http://www.w3.org/2003/05/soap-envelope\"")
	assert.Contains(t, xml, "xmlns:wsa=\"http://www.w3.org/2005/08/addressing\"")
	assert.Contains(t, xml, "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17")
}

func TestCreateAuthzDecisionQuery_Structure(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "123456789",
		HealthcareFacilityType: "H1",
		AuthorInstitutionID:    "00001234",
		EventCode:              "TEST001",
		SubjectRole:            "02.020",
		ProviderID:             "987654321",
		ProviderInstitutionID:  "00005678",
		ConsultingFacilityType: "H1",
		PurposeOfUse:           "RESEARCH",
	}

	xml, err := CreateAuthzDecisionQuery(req)
	require.NoError(t, err)

	// Verify the structure has all required categories
	assert.Contains(t, xml, "urn:oasis:names:tc:xacml:3.0:attribute-category:resource")
	assert.Contains(t, xml, "urn:oasis:names:tc:xacml:3.0:attribute-category:action")
	assert.Contains(t, xml, "urn:oasis:names:tc:xacml:1.0:subject-category:access-subject")
	assert.Contains(t, xml, "urn:oasis:names:tc:xacml:3.0:attribute-category:environment")

	// Verify xml:id attributes
	assert.Contains(t, xml, "xml:id=\"resource\"")
	assert.Contains(t, xml, "xml:id=\"action0\"")
	assert.Contains(t, xml, "xml:id=\"subject\"")
	assert.Contains(t, xml, "xml:id=\"environment\"")
}

func TestCreateAuthzDecisionQuery_AttributeIDs(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "111111111",
		HealthcareFacilityType: "Z1",
		AuthorInstitutionID:    "00000001",
		EventCode:              "CODE001",
		SubjectRole:            "01.001",
		ProviderID:             "000000001",
		ProviderInstitutionID:  "00000002",
		ConsultingFacilityType: "Z1",
		PurposeOfUse:           "TREAT",
	}

	xml, err := CreateAuthzDecisionQuery(req)
	require.NoError(t, err)

	// Verify AttributeId values are correct
	expectedAttributeIDs := []string{
		"urn:oasis:names:tc:xacml:2.0:resource:resource-id",
		"urn:ihe:iti:appc:2016:document-entry:healthcare-facility-type-code",
		"urn:ihe:iti:appc:2016:author-institution:id",
		"urn:ihe:iti:appc:2016:document-entry:event-code",
		"urn:oasis:names:tc:xacml:2.0:subject:role",
		"urn:ihe:iti:xua:2017:subject:provider-identifier",
		"urn:nl:otv:names:tc:1.0:subject:provider-institution",
		"urn:nl:otv:names:tc:1.0:subject:consulting-healthcare-facility-type-code",
		"urn:oasis:names:tc:xspa:1.0:subject:purposeofuse",
	}

	for _, attrID := range expectedAttributeIDs {
		assert.Contains(t, xml, attrID, "Missing AttributeId: %s", attrID)
	}
}

func TestCreateAuthzDecisionQuery_DataTypes(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "999999999",
		HealthcareFacilityType: "Z9",
		AuthorInstitutionID:    "00009999",
		EventCode:              "TEST999",
		SubjectRole:            "09.099",
		ProviderID:             "999999999",
		ProviderInstitutionID:  "00009998",
		ConsultingFacilityType: "Z9",
		PurposeOfUse:           "TREAT",
	}

	xml, err := CreateAuthzDecisionQuery(req)
	require.NoError(t, err)

	// Verify DataType attributes
	assert.True(t, strings.Contains(xml, "DataType=\"urn:hl7-org:v3#II\""),
		"Should contain InstanceIdentifier DataType")
	assert.True(t, strings.Contains(xml, "DataType=\"urn:hl7-org:v3#CV\""),
		"Should contain CodedValue DataType")

	// Verify HL7 v3 namespace
	assert.Contains(t, xml, "xmlns=\"urn:hl7-org:v3\"")
}

// generateTestSigningConfig creates a test signing configuration with a self-signed certificate
func generateTestSigningConfig(t *testing.T) *SigningConfig {
	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create a self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Organization"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	return &SigningConfig{
		PrivateKey:  privateKey,
		Certificate: [][]byte{certBytes},
	}
}

func TestCreateSignedAuthzDecisionQuery(t *testing.T) {
	// Create signing config
	signingConfig := generateTestSigningConfig(t)

	// Create request with values from example.xml
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
	require.NotEmpty(t, xml)

	// Verify signature elements are present
	assert.Contains(t, xml, "ds:Signature")
	assert.Contains(t, xml, "ds:SignedInfo")
	assert.Contains(t, xml, "ds:SignatureValue")
	assert.Contains(t, xml, "ds:KeyInfo")
	assert.Contains(t, xml, "ds:X509Data")
	assert.Contains(t, xml, "ds:X509Certificate")

	// Verify signature algorithms
	assert.Contains(t, xml, "http://www.w3.org/2001/10/xml-exc-c14n#")           // Canonicalization
	assert.Contains(t, xml, "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256") // Signature method
	assert.Contains(t, xml, "http://www.w3.org/2001/04/xmlenc#sha256")           // Digest method

	// Verify transforms
	assert.Contains(t, xml, "http://www.w3.org/2000/09/xmldsig#enveloped-signature")

	// Verify all original content is still present
	assert.Contains(t, xml, "900186021") // Patient BSN
	assert.Contains(t, xml, "GGC002")    // Event code
	assert.Contains(t, xml, "TREAT")     // Purpose of use
}

func TestCreateSignedAuthzDecisionQuery_WithoutConfig(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "123456789",
		HealthcareFacilityType: "Z3",
		AuthorInstitutionID:    "00000001",
		EventCode:              "TEST001",
		SubjectRole:            "01.015",
		ProviderID:             "000000001",
		ProviderInstitutionID:  "00000002",
		ConsultingFacilityType: "Z3",
		PurposeOfUse:           "TREAT",
	}

	_, err := CreateSignedAuthzDecisionQuery(req, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signing config is required")
}

func TestSignedQuery_HasRequestID(t *testing.T) {
	signingConfig := generateTestSigningConfig(t)

	req := AuthzRequest{
		PatientBSN:             "111111111",
		HealthcareFacilityType: "Z1",
		AuthorInstitutionID:    "00000001",
		EventCode:              "CODE001",
		SubjectRole:            "01.001",
		ProviderID:             "000000001",
		ProviderInstitutionID:  "00000002",
		ConsultingFacilityType: "Z1",
		PurposeOfUse:           "TREAT",
	}

	xml, err := CreateSignedAuthzDecisionQuery(req, signingConfig)
	require.NoError(t, err)

	// Verify Request has an xml:id attribute (required for signature reference)
	assert.Contains(t, xml, "xml:id=\"_")

	// Verify the signature references the Request ID
	assert.Contains(t, xml, "URI=\"#_")
}

func TestUnsignedQuery_NoSignature(t *testing.T) {
	req := AuthzRequest{
		PatientBSN:             "999999999",
		HealthcareFacilityType: "Z9",
		AuthorInstitutionID:    "00009999",
		EventCode:              "TEST999",
		SubjectRole:            "09.099",
		ProviderID:             "999999999",
		ProviderInstitutionID:  "00009998",
		ConsultingFacilityType: "Z9",
		PurposeOfUse:           "TREAT",
	}

	xml, err := CreateAuthzDecisionQuery(req)
	require.NoError(t, err)

	// Verify NO signature elements are present in unsigned query
	assert.NotContains(t, xml, "ds:Signature")
	assert.NotContains(t, xml, "ds:SignedInfo")
	assert.NotContains(t, xml, "ds:SignatureValue")
}

// ExampleCreateAuthzDecisionQuery demonstrates how to create an XACML authorization decision query.
// This example shows the typical usage pattern for generating SOAP envelopes containing
// XACML authorization decision queries based on the IHE APPC (Access Control) profile.
func ExampleCreateAuthzDecisionQuery() {
	// Create a request with all required parameters
	req := AuthzRequest{
		// Resource attributes (about what is being accessed)
		PatientBSN:             "900186021", // Patient's BSN
		HealthcareFacilityType: "Z3",        // Type of healthcare facility
		AuthorInstitutionID:    "00000659",  // Institution ID that created the document

		// Action attributes (what action is being requested)
		EventCode: "GGC002", // Event/procedure code

		// Subject attributes (who is requesting access)
		SubjectRole:            "01.015",    // Healthcare professional role
		ProviderID:             "000095254", // Healthcare provider ID
		ProviderInstitutionID:  "00000666",  // Institution of the provider
		ConsultingFacilityType: "Z3",        // Type of consulting facility

		// Environment attributes (context of the request)
		PurposeOfUse: "TREAT", // Purpose: TREAT, RESEARCH, etc.
	}

	// Generate the XACML query
	xml, err := CreateAuthzDecisionQuery(req)
	if err != nil {
		panic(err)
	}

	// The generated xml is a SOAP envelope containing an XACML authorization decision query
	// with proper namespace declarations and structured attributes for healthcare access control
	_ = xml // Use the generated XML for authorization decision requests
}
