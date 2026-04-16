package system.log_test

import rego.v1

import data.system.log

# The mask rule receives the full decision event; the original input sits at
# `input.input`. These fixtures mirror that structure.

base_event := {
	"path": "/bgz",
	"input": {
		"context": {},
		"subject": {},
		"action": {"request": {}, "fhir_rest": {}},
		"resource": {},
	},
}

# --- patient_bsn -----------------------------------------------------------

test_redacts_patient_bsn if {
	event := object.union(base_event, {"input": {"context": {"patient_bsn": "123456789"}}})
	{"op": "upsert", "path": "/input/context/patient_bsn", "value": "[REDACTED]"} in log.mask with input as event
}

test_no_patch_when_patient_bsn_absent if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/context/patient_bsn")
}

test_no_patch_when_patient_bsn_empty if {
	event := object.union(base_event, {"input": {"context": {"patient_bsn": ""}}})
	patches := log.mask with input as event
	not any_patch_for_path(patches, "/input/context/patient_bsn")
}

# --- patient_enrollment_identifier ----------------------------------------

test_redacts_patient_enrollment_identifier_uri_system if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|123456789"}}})
	{"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_redacts_patient_enrollment_identifier_oid_system if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": "urn:oid:2.16.840.1.113883.2.4.6.3|123456789"}}})
	{"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_keeps_non_bsn_enrollment_identifier_untouched if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/ura|12345"}}})
	patches := log.mask with input as event
	not any_patch_for_path(patches, "/input/subject/patient_enrollment_identifier")
}

# --- search_params.identifier ---------------------------------------------

test_redacts_search_params_identifier_uri_system if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"identifier": [["http://fhir.nl/fhir/NamingSystem/bsn|123456789"]]}}}}})
	{"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_redacts_search_params_identifier_oid_system if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"identifier": [["urn:oid:2.16.840.1.113883.2.4.6.3|123456789"]]}}}}})
	{"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_redacts_identifier_when_bsn_is_second_or_value if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"identifier": [["http://fhir.nl/fhir/NamingSystem/ura|12345", "http://fhir.nl/fhir/NamingSystem/bsn|123456789"]]}}}}})
	{"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_keeps_non_bsn_identifier_search_untouched if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"identifier": [["http://fhir.nl/fhir/NamingSystem/ura|12345"]]}}}}})
	patches := log.mask with input as event
	not any_patch_for_path(patches, "/input/action/fhir_rest/search_params/identifier")
}

test_keeps_other_search_params_untouched if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"_lastUpdated": [["2024-01-01"]]}}}}})
	patches := log.mask with input as event
	not any_patch_for_path(patches, "/input/action/fhir_rest/search_params/identifier")
}

# --- Wholesale redactions: query, body, header, resource.content ----------

test_redacts_request_query_when_present if {
	event := object.union(base_event, {"input": {"action": {"request": {"query": "identifier=whatever"}}}})
	{"op": "upsert", "path": "/input/action/request/query", "value": "[REDACTED]"} in log.mask with input as event
}

test_no_query_patch_when_query_empty if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/query")
}

test_redacts_request_body_when_present if {
	event := object.union(base_event, {"input": {"action": {"request": {"body": `{"identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"123456789"}]}`}}}})
	{"op": "upsert", "path": "/input/action/request/body", "value": "[REDACTED]"} in log.mask with input as event
}

test_no_body_patch_when_body_empty if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/body")
}

test_redacts_request_header_when_present if {
	event := object.union(base_event, {"input": {"action": {"request": {"header": {"Authorization": ["Bearer x"]}}}}})
	{"op": "upsert", "path": "/input/action/request/header", "value": "[REDACTED]"} in log.mask with input as event
}

test_no_header_patch_when_header_absent if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/header")
}

test_redacts_resource_content_when_present if {
	event := object.union(base_event, {"input": {"resource": {"content": {"resourceType": "Patient", "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "123456789"}]}}}})
	{"op": "upsert", "path": "/input/resource/content", "value": "[REDACTED]"} in log.mask with input as event
}

test_no_content_patch_when_content_absent if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/resource/content")
}

# --- Edge cases ------------------------------------------------------------

test_empty_event_produces_no_patches if {
	empty_event := {"path": "/bgz", "input": {}}
	patches := log.mask with input as empty_event
	count(patches) == 0
}

# Fail-closed: a non-string value where a string is expected must not raise a
# type error that causes OPA to fall back to logging the event unmasked.
test_non_string_enrollment_identifier_does_not_break_mask if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": {"unexpected": "shape"}}}})
	patches := log.mask with input as event

	# The enrollment identifier mask doesn't apply, but other rules still run;
	# what matters is that evaluation doesn't error.
	not any_patch_for_path(patches, "/input/subject/patient_enrollment_identifier")
}

test_non_string_search_params_identifier_does_not_break_mask if {
	event := object.union(base_event, {"input": {"action": {"fhir_rest": {"search_params": {"identifier": [[{"unexpected": "shape"}]]}}}}})
	patches := log.mask with input as event
	not any_patch_for_path(patches, "/input/action/fhir_rest/search_params/identifier")
}

# URN OID is case-insensitive per RFC 8141; a system URI uppercased should
# still be detected as BSN-scoped.
test_case_insensitive_oid_match if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": "URN:OID:2.16.840.1.113883.2.4.6.3|123456789"}}})
	{"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": "[REDACTED]"} in log.mask with input as event
}

test_case_insensitive_uri_match if {
	event := object.union(base_event, {"input": {"subject": {"patient_enrollment_identifier": "HTTP://FHIR.NL/FHIR/NAMINGSYSTEM/BSN|123456789"}}})
	{"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": "[REDACTED]"} in log.mask with input as event
}

# Empty header map should not emit a pointless patch.
test_empty_header_map_produces_no_patch if {
	event := object.union(base_event, {"input": {"action": {"request": {"header": {}}}}})
	patches := log.mask with input as event
	not "/input/action/request/header" in patches
}

# any_patch_for_path returns true if the mask set contains a patch (either the
# shorthand string form or an object form) targeting the given path.
any_patch_for_path(mask_set, path) if {
	path in mask_set
}

any_patch_for_path(mask_set, path) if {
	some patch in mask_set
	is_object(patch)
	patch.path == path
}
