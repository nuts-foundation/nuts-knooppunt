package lrza

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The building blocks the IG's profiles require of the addressing resources.
// https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/artifacts.html

// URANamingSystem is the URA identifier system the IG requires: the
// nl-gf-organization invariants only accept a URA under this system. The rest of
// the seed (NVI, PIP) still uses http://fhir.nl/fhir/NamingSystem/ura; only the
// addressing data has moved. Mirrors coding.URAOIDNamingSystem in the root
// module, which this module doesn't import.
const URANamingSystem = "urn:oid:2.16.528.1.1007.3.3"

const (
	kvkNamingSystem = "http://kvk.nl"
	// uraAssignerKVK is the KvK number the IG's example Organizations give as the
	// assigner of a URA identifier.
	uraAssignerKVK = "50000535"

	sbiSystem                       = "https://www.cbs.nl/standaard-bedrijfsindeling"
	dataCategorySystem              = "http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"
	fhirRESTConnectionSystem        = "http://terminology.hl7.org/CodeSystem/endpoint-connection-type"
	authorizationServerSystem       = "http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-authorization-server-cs"
	endpointPayloadTypeSystem       = "http://terminology.hl7.org/CodeSystem/endpoint-payload-type"
	fhirR4MimeType                  = "application/fhir+json; fhirVersion=4.0"
	organizationProfile             = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organization"
	endpointProfile                 = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-endpoint"
	healthcareServiceProfile        = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-healthcareservice"
	provenanceParticipantTypeSystem = "http://terminology.hl7.org/CodeSystem/provenance-participant-type"
)

// uraIdentifier is an organization's URA as the IG's AssignedId slice
// (nl-gf-custodianassignedidentifier) requires it: official, and naming the
// party that assigned it.
func uraIdentifier(ura string) fhir.Identifier {
	return fhir.Identifier{
		Use:      to.Ptr(fhir.IdentifierUseOfficial),
		System:   to.Ptr(URANamingSystem),
		Value:    to.Ptr(ura),
		Assigner: custodian(kvkNamingSystem, uraAssignerKVK),
	}
}

// custodian is the assigner of a custodian-assigned identifier. The IG only
// accepts a URA or KvK identifier here.
func custodian(system, value string) *fhir.Reference {
	return &fhir.Reference{Identifier: &fhir.Identifier{
		Type: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(provenanceParticipantTypeSystem),
			Code:   to.Ptr("custodian"),
		}}},
		System: to.Ptr(system),
		Value:  to.Ptr(value),
	}}
}

// sbiType is an Organization.type from the SBI (Standaard Bedrijfsindeling),
// which nl-gf-org-types-vs includes.
func sbiType(code, display string) []fhir.CodeableConcept {
	return []fhir.CodeableConcept{{Coding: []fhir.Coding{{
		System:  to.Ptr(sbiSystem),
		Code:    to.Ptr(code),
		Display: to.Ptr(display),
	}}}}
}

// managedBy is the Endpoint.managingOrganization the IG requires. In this demo
// every organization manages its own endpoints.
func managedBy(organizationID string) *fhir.Reference {
	return &fhir.Reference{Reference: to.Ptr("Organization/" + organizationID), Type: to.Ptr("Organization")}
}

// dataCategories is a FHIR endpoint's payloadType: the nl-gf-data-categories-cs
// categories it serves.
func dataCategories(codes ...string) []fhir.CodeableConcept {
	var result []fhir.CodeableConcept
	for _, code := range codes {
		result = append(result, fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(dataCategorySystem),
			Code:   to.Ptr(code),
		}}})
	}
	return result
}

// noPayload is the payloadType of an authorization-server endpoint. payloadType
// is 1..* in R4, and the IG defines no data category for an authorization
// server; endpoint-payload-type is part of nl-gf-payload-type-vs.
func noPayload() []fhir.CodeableConcept {
	return []fhir.CodeableConcept{{Coding: []fhir.Coding{{
		System:  to.Ptr(endpointPayloadTypeSystem),
		Code:    to.Ptr("none"),
		Display: to.Ptr("None"),
	}}}}
}

// fhirRESTConnection and oauthNutsConnection are the connectionTypes of a FHIR
// endpoint and of a Nuts authorization server, both in nl-gf-connection-types-vs.
func fhirRESTConnection() fhir.Coding {
	return fhir.Coding{System: to.Ptr(fhirRESTConnectionSystem), Code: to.Ptr("hl7-fhir-rest")}
}

func oauthNutsConnection() fhir.Coding {
	return fhir.Coding{System: to.Ptr(authorizationServerSystem), Code: to.Ptr("oauth-nuts")}
}
