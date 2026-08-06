package main

// nutsConfig separates the URL this process calls from the URL the node
// advertises as its own issuer. In compose they differ: the sandbox reaches
// the node by service name, while the issuer must stay exactly what the node
// publishes, because the audience check compares the two strings verbatim.
type nutsConfig struct {
	InternalBaseURL     string
	Subject             string
	Scope               string
	AuthorizationServer string
	FacilityType        string
}

func nutsConfigFromEnv() nutsConfig {
	return nutsConfig{
		InternalBaseURL:     envOr("NUTS_INTERNAL_BASE_URL", "http://localhost:8081"),
		Subject:             envOr("SANDBOX_NUTS_SUBJECT", "plataan"),
		Scope:               envOr("SANDBOX_BGZ_SCOPE", "bgz"),
		AuthorizationServer: envOr("SANDBOX_AUTH_SERVER", "http://localhost:8080/nuts/oauth2/plataan"),
		FacilityType:        envOr("SANDBOX_FACILITY_TYPE", "Z3"),
	}
}

// organizationContextCredential carries the one claim nothing else on this
// path supplies. The UZI certificate's SAN holds the URA and the Dezi
// attestation holds the practitioner, but neither knows a facility type.
//
// It deliberately sets no issuer and no credentialSubject.id: the token
// endpoint rejects a request credential carrying either, and the node's client
// fills both in from the requester's DID before the credential is verified.
//
// This is a caller assertion. It demonstrates the plumbing, not organizational
// assurance, and nothing ties its facility type to the URA the X509 credential
// carries.
func organizationContextCredential(cfg nutsConfig) map[string]any {
	return map[string]any{
		"@context":          []string{"https://www.w3.org/2018/credentials/v1"},
		"type":              []string{"VerifiableCredential", "SandboxOrganizationContextCredential"},
		"credentialSubject": map[string]any{"facilityType": cfg.FacilityType},
	}
}
