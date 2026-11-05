package profile

const NLGenericFunctionOrganization = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"
const NLGenericFunctionEndpoint = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"
const NLGenericFunctionLocation = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-location"
const NLGenericFunctionHealthcareService = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-healthcareservice"
const NLGenericFunctionDocumentReference = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-localization-documentreference"

// The GF Care Service Directory (CSD) 1.0.0 profiles, which the LRZA validates addressing resources
// against. The NLGenericFunction* canonicals above predate the CSD IG and are still used by mcsdadmin.
// https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/artifacts.html
const CSDOrganization = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organization"
const CSDLocation = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-location"
const CSDHealthcareService = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-healthcareservice"
const CSDEndpoint = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-endpoint"
