package medicatieoverdracht_gf_test

import rego.v1

import data.medicatieoverdracht_gf

base_input := {
	"action": {"fhir_rest": {
		"capability_checked": true,
		"interaction_type": "search-type",
	}},
	"context": {
		"mitz_consent": true,
		"patient_id": "patient-123",
		"patient_bsn": "999999999",
	},
	"resource": {"type": "MedicationRequest"},
	"subject": {"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|999999999"},
}

base_patient_input := {
	"action": {"fhir_rest": {
		"capability_checked": true,
		"interaction_type": "search-type",
		"search_params": {"identifier": [["http://fhir.nl/fhir/NamingSystem/bsn|999999999"]]},
	}},
	"context": {
		"mitz_consent": true,
		"patient_bsn": "999999999",
	},
	"resource": {"type": "Patient"},
	"subject": {"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|999999999"},
}

# Direct rule tests for requester_has_enrolled_patient (complex BSN concatenation logic)

test_requester_has_enrolled_patient_valid if {
	medicatieoverdracht_gf.requester_has_enrolled_patient with input as base_input
}

test_requester_has_enrolled_patient_bsn_mismatch if {
	not medicatieoverdracht_gf.requester_has_enrolled_patient with input as object.union(base_input, {
		"subject": {"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|111111111"},
	})
}

# Allow/deny through allow

test_allow_medication_request_search if {
	medicatieoverdracht_gf.allow with input as base_input
}

test_deny_without_capability_checked if {
	not medicatieoverdracht_gf.allow with input as object.union(base_input, {
		"action": {"fhir_rest": {"capability_checked": false}},
	})
}

test_deny_without_mitz_consent if {
	not medicatieoverdracht_gf.allow with input as object.union(base_input, {"context": {"mitz_consent": false}})
}

test_deny_without_patient_identifier if {
	not medicatieoverdracht_gf.allow with input as object.union(base_input, {
		"context": {"patient_id": "", "patient_bsn": ""},
	})
}

test_allow_with_patient_bsn_identifier if {
	medicatieoverdracht_gf.allow with input as object.union(base_input, {"context": {"patient_id": ""}})
}

test_allow_patient_bsn_search if {
	medicatieoverdracht_gf.allow with input as base_patient_input
}

test_deny_patient_search_without_mitz_consent if {
	not medicatieoverdracht_gf.allow with input as object.union(base_patient_input, {"context": {"mitz_consent": false, "patient_bsn": "999999999"}})
}

test_deny_patient_search_empty_bsn if {
	not medicatieoverdracht_gf.allow with input as object.union(base_patient_input, {"context": {"mitz_consent": true, "patient_bsn": ""}})
}

test_deny_patient_search_wrong_identifier_system if {
	not medicatieoverdracht_gf.allow with input as object.union(base_patient_input, {
		"action": {"fhir_rest": {"capability_checked": true, "interaction_type": "search-type", "search_params": {"identifier": [["http://other-system|999999999"]]}}},
	})
}

test_deny_wrong_interaction_type if {
	not medicatieoverdracht_gf.allow with input as object.union(base_input, {
		"action": {"fhir_rest": {"interaction_type": "read"}},
	})
}
