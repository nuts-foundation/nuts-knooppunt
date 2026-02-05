package medicatieoverdracht

import rego.v1

#
# This file implements the FHIR queries for the Medicatieoverdracht use case, as specified by https://informatiestandaarden.nictiz.nl/wiki/MedMij:V2019.01_FHIR_MedicationProcess
#
# But, we only allow searchinf for MedicationRequest for now

default allow := false
default msg := ""

allow if {
    input.context.fhir_capability_checked == true
    input.context.mitz_consent == true
    # The BgZ on Generic Functions use case (to be formalized) specifies that requests must be scoped to a patient.
    # We enforce this by checking that either a patient_id or patient_bsn is present in the request context.
    has_patient_identifier
    requester_has_enrolled_patient
    is_allowed_query
}

# Helper rule: check if either patient_id or patient_bsn is filled
has_patient_identifier if {
    is_string(input.context.patient_id)
    input.context.patient_id != ""
}

has_patient_identifier if {
    # Remove this after february 2026 hackaton.
    is_string(input.context.patient_bsn)
    input.context.patient_bsn != ""
}

# This rule checks whether the requesting party actually has the patient in care.
requester_has_enrolled_patient if {
    input.context.patient_bsn == concat("http://fhir.nl/fhir/NamingSystem/bsn|", input.action.properties.patient_enrollment_identifier)
}

# GET [base]/MedicationRequest
is_allowed_query if {
    input.resource.type == "MedicationRequest"
    input.action.properties.interaction_type == "search-type"
}

# Collect all failure reasons
reason[msg] if {
    input.context.fhir_capability_checked != true
    msg := "FHIR capability not validated"
}

reason[msg] if {
    input.context.mitz_consent != true
    msg := "MedMij consent not granted"
}

reason[msg] if {
    not has_patient_identifier
    msg := "No patient identifier provided (patient_id or patient_bsn required)"
}

reason[msg] if {
    not requester_has_enrolled_patient
    msg := "Requester does not have patient enrolled"
}

reason[msg] if {
    not is_allowed_query
    msg := "FHIR query not allowed"
}
