package xacml

import "crypto/rsa"

// AuthzRequest represents the parameters needed to create an XACML authorization decision query
type AuthzRequest struct {
	// PatientBSN is the patient BSN (extension value)
	PatientBSN string
	// HealthcareFacilityType is the healthcare facility type (e.g., "Z3")
	HealthcareFacilityType string
	// AuthorInstitutionID is the author institution ID (e.g., "00000659")
	AuthorInstitutionID string

	// EventCode is the action event code (e.g., "GGC002")
	EventCode string

	// SubjectRole is the subject role (e.g., "01.015")
	SubjectRole string
	// ProviderID is the provider ID (e.g., "000095254")
	ProviderID string
	// ProviderInstitutionID is the provider institution ID (e.g., "00000666")
	ProviderInstitutionID string
	// ConsultingFacilityType is the consulting facility type (e.g., "Z3")
	ConsultingFacilityType string

	// PurposeOfUse is the purpose of use (e.g., "TREAT")
	PurposeOfUse string

	// ToAddress is the endpoint address (e.g., "http://localhost:8000/4")
	ToAddress string
}

// SigningConfig contains the configuration for signing XACML queries
type SigningConfig struct {
	// PrivateKey for signing the request
	PrivateKey *rsa.PrivateKey
	// Certificate chain (leaf first)
	Certificate [][]byte
}
