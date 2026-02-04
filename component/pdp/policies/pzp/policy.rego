package pzp

import rego.v1

#
# This file implements the FHIR queries for PZP/ACP (Proactieve ZorgPlanning)
# as specified by https://wiki.nuts.nl/books/pzp/page/pzp-volume-3-content
#

default allow := false

allow if {
    input.context.mitz_consent == true
    is_allowed_query
}

# GET [base]/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|{value}
is_allowed_query if {
    input.resource.type == "Patient"
    input.action.properties.connection_data.fhir_rest.interaction_type == "search-type"
    # identifier: exactly 1 identifier of type BSN
    is_string(input.action.properties.connection_data.fhir_rest.search_params.identifier)
    startswith(input.action.properties.connection_data.fhir_rest.search_params.identifier, "http://fhir.nl/fhir/NamingSystem/bsn|")
}

# GET [base]/Consent?patient={reference}&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2
is_allowed_query if {
    input.resource.type == "Consent"
    input.action.properties.connection_data.fhir_rest.interaction_type == "search-type"
    # patient: reference Patient resource
    startswith(input.action.properties.connection_data.fhir_rest.search_params.patient, "Patient/")
    # _profile
    is_string(input.action.properties.connection_data.fhir_rest.search_params._profile)
    input.action.properties.connection_data.fhir_rest.search_params._profile == "http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2"
}
