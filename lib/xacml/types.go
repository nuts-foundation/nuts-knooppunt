package xacml

import "crypto/rsa"

// AuthzRequest represents the parameters needed to create an XACML authorization decision query
type AuthzRequest struct {
	// Resource attributes
	PatientBSN             string // Patient BSN (extension value)
	HealthcareFacilityType string // e.g., "Z3"
	AuthorInstitutionID    string // e.g., "00000659"

	// Action attributes
	EventCode string // e.g., "GGC002"

	// Subject attributes
	SubjectRole            string // e.g., "01.015"
	ProviderID             string // e.g., "000095254"
	ProviderInstitutionID  string // e.g., "00000666"
	ConsultingFacilityType string // e.g., "Z3"

	// Environment attributes
	PurposeOfUse string // e.g., "TREAT"

	// Endpoint
	ToAddress string // e.g., "http://localhost:8000/4"
}

// SigningConfig contains the configuration for signing XACML queries
type SigningConfig struct {
	// PrivateKey for signing the request
	PrivateKey *rsa.PrivateKey
	// Certificate chain (leaf first)
	Certificate [][]byte
}
