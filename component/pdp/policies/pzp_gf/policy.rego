package pzp_gf

import rego.v1

#
# This file implements the FHIR queries for PZP/ACP (Proactieve ZorgPlanning)
# as specified by https://wiki.nuts.nl/books/pzp/page/pzp-volume-3-content
#

default allow := false
allow if {
    patient_gave_mitz_consent
    is_allowed_query
}

default patient_gave_mitz_consent := false
patient_gave_mitz_consent if {
    input.context.mitz_consent == true
}

# GET [base]/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|{value}
default is_allowed_query := false
is_allowed_query if {
    input.resource.type == "Patient"
    input.action.fhir_rest.interaction_type == "search-type"
    # identifier: exactly 1 identifier of type BSN
    is_string(input.action.fhir_rest.search_params.identifier)
    startswith(input.action.fhir_rest.search_params.identifier, "http://fhir.nl/fhir/NamingSystem/bsn|")
}

# GET [base]/Consent?patient={reference}&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009
is_allowed_query if {
    input.resource.type == "Consent"
    input.action.fhir_rest.interaction_type == "search-type"
    # patient: reference Patient resource
    startswith(input.action.fhir_rest.search_params.patient, "Patient/")
    input.action.fhir_rest.search_params.scope == "http://terminology.hl7.org/CodeSystem/consentscope|treatment"
    input.action.fhir_rest.search_params.category == "http://snomed.info/sct|129125009"
}
