package xacml

import "crypto/rsa"

// Decision represents the XACML authorization decision
type Decision string

const (
	// DecisionPermit indicates that access is permitted
	DecisionPermit Decision = "Permit"
	// DecisionDeny indicates that access is denied
	DecisionDeny Decision = "Deny"
	// DecisionNotApplicable indicates that no applicable policy was found
	DecisionNotApplicable Decision = "NotApplicable"
	// DecisionIndeterminate indicates that the decision could not be determined
	DecisionIndeterminate Decision = "Indeterminate"
)

// String returns the string representation of the decision
func (d Decision) String() string {
	return string(d)
}

// XACMLResponse represents the parsed XACML authorization decision query response
type XACMLResponse struct {
	// Decision is the authorization decision (Permit, Deny, NotApplicable, or Indeterminate)
	Decision Decision
	// RawXML contains the full XML response
	RawXML []byte
}

// AuthzRequest represents the parameters needed to create an XACML authorization decision query.
// This struct is used when invoking the CheckConsent function to perform a consent check with the MITZ system.
// Note: The callback address (ToAddress) is hardcoded to "http://localhost:8000/4" in the generated XACML XML.
//
// Example usage:
//
//	req := xacml.AuthzRequest{
//	    PatientBSN:             "900186021",
//	    HealthcareFacilityType: "Z3",
//	    AuthorInstitutionID:    "00000659",
//	    EventCode:              "GGC002",
//	    SubjectRole:            "01.015",
//	    ProviderID:             "000095254",
//	    ProviderInstitutionID:  "00000666",
//	    ConsultingFacilityType: "Z3",
//	    PurposeOfUse:           "TREAT",
//	}
//	response, err := component.CheckConsent(ctx, req)
type AuthzRequest struct {
	// PatientBSN is the patient's BSN (Burgerservicenummer - Dutch citizen service number).
	// REQUIRED - The extension value uniquely identifying the patient.
	// Example: "900186021"
	PatientBSN string

	// HealthcareFacilityType is the healthcare facility type classification code.
	// REQUIRED - Identifies the type of healthcare facility.
	// Valid values: see https://decor.nictiz.nl/pub/eoverdracht/e-overdracht-html-20120928T120000/vs-2.16.840.1.113883.2.4.15.1060.html
	// Example: "Z3" (general hospital code)
	HealthcareFacilityType string

	// AuthorInstitutionID is the unique identifier (URA code) of the individual healthcare provider institution
	// that is filing/authoring the authorization request.
	// REQUIRED - Identifies the filing organization requesting the authorization.
	// Example: "00000659" (URA code)
	AuthorInstitutionID string

	// EventCode is the event action code that defines the type of consent decision being requested.
	// values from: 2.16.840.1.113883.2.4.3.111.5.10.1, example: GGC007
	EventCode string

	// SubjectRole is the UZI role code of the subject (healthcare professional) making the request.
	// REQUIRED - Must be a valid role code.
	// Valid values: see https://decor.nictiz.nl/pub/eoverdracht/e-overdracht-html-20120928T120000/vs-2.16.840.1.113883.2.4.15.111.html
	// Example: "01.015" (physician role code)
	SubjectRole string

	// ProviderID Identification number responsible
	// Example: "000095254" (UZI number)
	ProviderID string

	// ProviderInstitutionID is the unique identifier (URA code) of the healthcare institution (consulting organization)
	// REQUIRED - Identifies the consulting/responsible organization.
	// Example: "00000666" (institution URA code)
	ProviderInstitutionID string

	// ConsultingFacilityType is the consulting facility type classification code.
	// REQUIRED - Identifies the type of facility providing the consultation/service.
	// Valid values: see https://decor.nictiz.nl/pub/eoverdracht/e-overdracht-html-20120928T120000/vs-2.16.840.1.113883.2.4.15.1060.html
	// Example: "Z3" (general hospital code)
	ConsultingFacilityType string

	// PurposeOfUse is the reason/purpose for accessing the patient's information.
	// REQUIRED - Indicates the intended use of the authorization.
	// Valid values: "TREAT" (Normal with explicit consent) or "COC" (Normal with assumed consent)
	// Example: "TREAT"
	PurposeOfUse string
}

// SigningConfig contains the configuration for signing XACML queries
type SigningConfig struct {
	// PrivateKey for signing the request
	PrivateKey *rsa.PrivateKey
	// Certificate chain (leaf first)
	Certificate [][]byte
}
