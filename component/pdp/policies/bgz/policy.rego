package bgz

import rego.v1

#
# This file implements the FHIR queries as specified by https://informatiestandaarden.nictiz.nl/wiki/MedMij:V2020.01/FHIR_BGZ_2017
#

default allow := false

allow if {
    input.action.fhir_rest.capability_checked == true
    input.context.mitz_consent == true
    # The BgZ on Generic Functions use case (to be formalized) specifies that requests must be scoped to a patient.
    # We enforce this by checking that either a patient_id or patient_bsn is present in the request context.
    has_patient_identifier
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

# GET [base]/Patient?_include=Patient:general-practitioner
is_allowed_query if {
    input.resource.type == "Patient"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.include == ["Patient:general-practitioner"]
}

# GET [base]/Coverage?_include=Coverage:payor:Patient&_include=Coverage:payor:Organization
is_allowed_query if {
    input.resource.type == "Coverage"
    input.action.fhir_rest.interaction_type == "search-type"
    {e | some e in input.action.fhir_rest.include} == {"Coverage:payor:Patient", "Coverage:payor:Organization"}
}

# GET [base]/Consent?category=http://snomed.info/sct|11291000146105
is_allowed_query if {
    input.resource.type == "Consent"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|11291000146105"]
}

# GET [base]/Consent?category=http://snomed.info/sct|11341000146107
is_allowed_query if {
    input.resource.type == "Consent"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|11341000146107"]
}

# GET [base]/Observation/$lastn?category=http://snomed.info/sct|118228005,http://snomed.info/sct|384821006
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.operation == "$lastn"
    input.action.fhir_rest.interaction_type == "operation"
    {e | some e in input.action.fhir_rest.search_params.category} == {"http://snomed.info/sct|118228005", "http://snomed.info/sct|384821006"}
}

# GET [base]/Condition
is_allowed_query if {
    input.resource.type == "Condition"
    input.action.fhir_rest.interaction_type == "search-type"
}

# GET [base]/Observation/$lastn?code=http://snomed.info/sct|365508006
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.operation == "$lastn"
    input.action.fhir_rest.interaction_type == "operation"
    input.action.fhir_rest.search_params.code == ["http://snomed.info/sct|365508006"]
}

# GET [base]/Observation?code=http://snomed.info/sct|228366006
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.code == ["http://snomed.info/sct|228366006"]
}

# GET [base]/Observation?code=http://snomed.info/sct|228273003
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.code == ["http://snomed.info/sct|228273003"]
}

# GET [base]/Observation?code=http://snomed.info/sct|365980008
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.code == ["http://snomed.info/sct|365980008"]
}

# GET [base]/NutritionOrder
is_allowed_query if {
    input.resource.type == "NutritionOrder"
    input.action.fhir_rest.interaction_type == "search-type"
}

# GET [base]/Flag
is_allowed_query if {
    input.resource.type == "Flag"
    input.action.fhir_rest.interaction_type == "search-type"
}

# GET [base]/AllergyIntolerance
is_allowed_query if {
    input.resource.type == "AllergyIntolerance"
    input.action.fhir_rest.interaction_type == "search-type"
}

# GET [base]/MedicationStatement?category=urn:oid:2.16.840.1.113883.2.4.3.11.60.20.77.5.3|6&_include=MedicationStatement:medication
is_allowed_query if {
    input.resource.type == "MedicationStatement"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["urn:oid:2.16.840.1.113883.2.4.3.11.60.20.77.5.3|6"]
    input.action.fhir_rest.include == ["MedicationStatement:medication"]
}

# GET [base]/MedicationRequest?category=http://snomed.info/sct|16076005&_include=MedicationRequest:medication
is_allowed_query if {
    input.resource.type == "MedicationRequest"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|16076005"]
    input.action.fhir_rest.include == ["MedicationRequest:medication"]
}

# GET [base]/MedicationDispense?category=http://snomed.info/sct|422037009&_include=MedicationDispense:medication
is_allowed_query if {
    input.resource.type == "MedicationDispense"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|422037009"]
    input.action.fhir_rest.include == ["MedicationDispense:medication"]
}

# GET [base]/DeviceUseStatement?_include=DeviceUseStatement:device
is_allowed_query if {
    input.resource.type == "DeviceUseStatement"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.include == ["DeviceUseStatement:device"]
}

# GET [base]/Immunization?status=completed
is_allowed_query if {
    input.resource.type == "Immunization"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.status == ["completed"]
}

# GET [base]/Observation/$lastn?code=http://loinc.org|85354-9
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.operation == "$lastn"
    input.action.fhir_rest.interaction_type == "operation"
    input.action.fhir_rest.search_params.code == ["http://loinc.org|85354-9"]
}

# GET [base]/Observation/$lastn?code=http://loinc.org|8302-2,http://loinc.org|8306-3,http://loinc.org|8308-9
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.operation == "$lastn"
    input.action.fhir_rest.interaction_type == "operation"
    {e | some e in input.action.fhir_rest.search_params.code} == {"http://loinc.org|8302-2", "http://loinc.org|8306-3", "http://loinc.org|8308-9"}
}

# GET [base]/Observation/$lastn?category=http://snomed.info/sct|275711006&_include=Observation:related-target&_include=Observation:specimen
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.operation == "$lastn"
    input.action.fhir_rest.interaction_type == "operation"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|275711006"]
    {e | some e in input.action.fhir_rest.include} == {"Observation:related-target", "Observation:specimen"}
}

# GET [base]/Procedure?category=http://snomed.info/sct|387713003
is_allowed_query if {
    input.resource.type == "Procedure"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == ["http://snomed.info/sct|387713003"]
}

# GET [base]/Encounter?class=http://hl7.org/fhir/v3/ActCode|IMP,http://hl7.org/fhir/v3/ActCode|ACUTE,http://hl7.org/fhir/v3/ActCode|NONAC
is_allowed_query if {
    input.resource.type == "Encounter"
    input.action.fhir_rest.interaction_type == "search-type"
    {e | some e in input.action.fhir_rest.search_params.class} == {"http://hl7.org/fhir/v3/ActCode|IMP", "http://hl7.org/fhir/v3/ActCode|ACUTE", "http://hl7.org/fhir/v3/ActCode|NONAC"}
}

# GET [base]/ProcedureRequest?status=active
is_allowed_query if {
    input.resource.type == "ProcedureRequest"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.status == ["active"]
}

# GET [base]/ImmunizationRecommendation
is_allowed_query if {
    input.resource.type == "ImmunizationRecommendation"
    input.action.fhir_rest.interaction_type == "search-type"
}

# GET [base]/MedicationDispense?category=http://snomed.info/sct|422037009&status=in-progress,preparation&_include=MedicationDispense:medication
# This FHIR query was removed from spec

# GET [base]/DeviceRequest?status=active&_include=DeviceRequest:device
is_allowed_query if {
    input.resource.type == "DeviceRequest"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.status == ["active"]
    input.action.fhir_rest.include == ["DeviceRequest:device"]
}

# GET [base]/Appointment?status=booked,pending,proposed
is_allowed_query if {
    input.resource.type == "Appointment"
    input.action.fhir_rest.interaction_type == "search-type"
    {e | some e in input.action.fhir_rest.search_params.status} == {"booked", "pending", "proposed"}
}

# GET [base]/DocumentReference?status=current
is_allowed_query if {
    input.resource.type == "DocumentReference"
    input.action.properties.interaction_type == "search-type"
    input.action.properties.search_params.status == ["current"]
}

# GET [base]/DocumentReference
is_allowed_query if {
    input.resource.type == "DocumentReference"
    input.action.properties.interaction_type == "read"
}