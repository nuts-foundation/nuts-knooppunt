package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The building blocks the GF Care Service Directory (CSD) IG requires of addressing resources.
// https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/artifacts.html

// KVKOIDNamingSystem is the KvK identifier system the CSD IG uses for the assigner of a
// custodian-assigned identifier (e.g. the party that assigned a URA). It differs from
// KVKNamingSystem, which the rest of the Knooppunt uses.
const KVKOIDNamingSystem = "http://kvk.nl"

// DefaultURAAssignerKVK is the KvK number the IG's example Organizations give as the assigner of a
// URA identifier. Deployments can override it (see lrza.registration.uraassignerkvk).
const DefaultURAAssignerKVK = "50000535"

// RFC3986NamingSystem is the identifier system of a custodian-assigned identifier whose value is a
// URI, such as a urn:uuid.
const RFC3986NamingSystem = "urn:ietf:rfc:3986"

const (
	ProvenanceParticipantTypeSystem = "http://terminology.hl7.org/CodeSystem/provenance-participant-type"
	EndpointConnectionTypeSystem    = "http://terminology.hl7.org/CodeSystem/endpoint-connection-type"
	EndpointPayloadTypeSystem       = "http://terminology.hl7.org/CodeSystem/endpoint-payload-type"
	CSDAuthorizationServerSystem    = "http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-authorization-server-cs"
	CSDDataCategoriesSystem         = "http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"
	// SignatureTypeSystem is the code system of Provenance.signature.type.
	SignatureTypeSystem = "urn:iso-astm:E1762-95:2013"
	// AuthorSignatureCode is the SignatureTypeSystem code of an author's signature.
	AuthorSignatureCode = "1.2.840.10065.1.12.1.1"
	// FHIRR4JSONMimeType is the Endpoint.payloadMimeType of a FHIR R4 JSON endpoint.
	FHIRR4JSONMimeType = "application/fhir+json; fhirVersion=4.0"
)

// URAIdentifier is an organization's URA as the IG's AssignedId slice
// (nl-gf-custodianassignedidentifier) requires it: official, under the URA OID system, and naming
// the party that assigned it (by KvK number).
func URAIdentifier(ura, assignerKVK string) fhir.Identifier {
	return fhir.Identifier{
		Use:      to.Ptr(fhir.IdentifierUseOfficial),
		System:   to.Ptr(URAOIDNamingSystem),
		Value:    to.Ptr(ura),
		Assigner: Custodian(KVKOIDNamingSystem, assignerKVK),
	}
}

// Custodian is the assigner of a custodian-assigned identifier. The IG only accepts a URA or KvK
// identifier here.
func Custodian(system, value string) *fhir.Reference {
	return &fhir.Reference{Identifier: &fhir.Identifier{
		Type: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(ProvenanceParticipantTypeSystem),
			Code:   to.Ptr("custodian"),
		}}},
		System: to.Ptr(system),
		Value:  to.Ptr(value),
	}}
}
