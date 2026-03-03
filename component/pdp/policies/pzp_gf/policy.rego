package pzp_gf

import rego.v1
import data.fhir

#
# This file implements the FHIR queries for PZP/ACP (Proactieve ZorgPlanning)
# as specified by https://wiki.nuts.nl/books/pzp/page/pzp-volume-3-content
#

default allow := false
allow if {
    request_conforms_fhir_capabilitystatement
    patient_gave_mitz_consent
    is_allowed_query
}

default request_conforms_fhir_capabilitystatement := false
request_conforms_fhir_capabilitystatement if {
    fhir.capability_statement_allowed(input.capability_statement, input.resource.type, input.action.fhir_rest)
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
    is_string(input.context.patient_bsn)
    input.context.patient_bsn != ""
    startswith(input.action.fhir_rest.search_params.identifier[0], "http://fhir.nl/fhir/NamingSystem/bsn|")
}

# GET [base]/Consent?patient={reference}&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009
is_allowed_query if {
    input.resource.type == "Consent"
    input.action.fhir_rest.interaction_type == "search-type"
    # patient: reference Patient resource
    is_string(input.context.patient_id)
    input.context.patient_id != ""
    startswith(input.action.fhir_rest.search_params.patient[0], "Patient/")
    input.action.fhir_rest.search_params.scope == ["http://terminology.hl7.org/CodeSystem/consentscope|treatment"]
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|129125009"]
}
