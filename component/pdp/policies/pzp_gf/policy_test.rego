package pzp_gf_test

import rego.v1

import data.pzp_gf

base_patient_input := {
	"action": {"fhir_rest": {
		"interaction_type": "search-type",
		"search_params": {"identifier": [["http://fhir.nl/fhir/NamingSystem/bsn|123456789"]]},
	}},
	"context": {
		"mitz_consent": true,
		"patient_bsn": "123456789",
	},
	"resource": {"type": "Patient"},
}

base_consent_input := {
	"action": {"fhir_rest": {
		"interaction_type": "search-type",
		"search_params": {
			"patient": [["Patient/123"]],
			"scope": [["http://terminology.hl7.org/CodeSystem/consentscope|treatment"]],
			"category": [["http://snomed.info/sct|129125009"]],
		},
	}},
	"context": {
		"mitz_consent": true,
		"patient_id": "123",
	},
	"resource": {"type": "Consent"},
}

# Patient BSN search

test_allow_patient_bsn_search if {
	pzp_gf.allow with input as base_patient_input
}

test_deny_patient_search_without_mitz_consent if {
	not pzp_gf.allow with input as object.union(base_patient_input, {"context": {"mitz_consent": false}})
}

test_deny_patient_search_empty_bsn if {
	not pzp_gf.allow with input as object.union(base_patient_input, {"context": {"patient_bsn": ""}})
}

test_deny_patient_search_wrong_identifier_system if {
	not pzp_gf.allow with input as object.union(base_patient_input, {
		"action": {"fhir_rest": {"search_params": {"identifier": [["http://other-system|123456789"]]}}},
	})
}

# Consent search

test_allow_consent_search if {
	pzp_gf.allow with input as base_consent_input
}

test_deny_consent_search_without_patient_id if {
	not pzp_gf.allow with input as object.union(base_consent_input, {"context": {"patient_id": ""}})
}

test_deny_consent_search_wrong_scope if {
	not pzp_gf.allow with input as object.union(base_consent_input, {
		"action": {"fhir_rest": {"search_params": {"scope": [["http://terminology.hl7.org/CodeSystem/consentscope|research"]]}}},
	})
}

test_deny_consent_search_wrong_category if {
	not pzp_gf.allow with input as object.union(base_consent_input, {
		"action": {"fhir_rest": {"search_params": {"category": [["http://snomed.info/sct|999999"]]}}},
	})
}

test_deny_consent_search_patient_ref_no_prefix if {
	not pzp_gf.allow with input as object.union(base_consent_input, {
		"action": {"fhir_rest": {"search_params": {"patient": [["123"]]}}},
	})
}

test_deny_wrong_resource_type if {
	not pzp_gf.allow with input as object.union(base_patient_input, {"resource": {"type": "Observation"}})
}
