package medicatieoverdracht

import rego.v1

#
# This file implements the FHIR queries for the Medicatieoverdracht use case, as specified by https://informatiestandaarden.nictiz.nl/wiki/MedMij:V2019.01_FHIR_MedicationProcess
#
# But, we only allow searching for MedicationRequest for now.
# TODO/Warning: the MedicationRequest query rule might need additional checks on search parameters (search narrowing).

default allow := false
allow if {
    request_conforms_fhir_capabilitystatement
    patient_gave_mitz_consent
    # The MedicatieOverdracht requests must be scoped to a patient.
    # We enforce this by checking that either a patient_id or patient_bsn is present in the request context.
    has_patient_identifier
    requester_has_enrolled_patient
    is_allowed_query
}

default request_conforms_fhir_capabilitystatement := false
request_conforms_fhir_capabilitystatement if {
    input.action.fhir_rest.capability_checked == true
}

default patient_gave_mitz_consent := false
patient_gave_mitz_consent if {
    input.context.mitz_consent == true
}

# Helper rule: check if either patient_id or patient_bsn is filled
default has_patient_identifier := false
has_patient_identifier if {
    is_string(input.context.patient_id)
    input.context.patient_id != ""
}

has_patient_identifier if {
    # Remove this after february 2026 hackathon.
    is_string(input.context.patient_bsn)
    input.context.patient_bsn != ""
}

# This rule checks whether the requesting party actually has the patient in care.
default requester_has_enrolled_patient := false
requester_has_enrolled_patient if {
    # We must have a patient BSN
    not input.context.patient_bsn == ""
    is_string(input.context.patient_bsn)
    concat("", ["http://fhir.nl/fhir/NamingSystem/bsn|", input.context.patient_bsn]) == input.subject.properties.patient_enrollment_identifier
}

# GET [base]/MedicationRequest
default is_allowed_query := false
is_allowed_query if {
    input.resource.type == "MedicationRequest"
    input.action.fhir_rest.interaction_type == "search-type"
}