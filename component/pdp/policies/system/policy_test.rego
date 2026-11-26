package system.log_test

import rego.v1

import data.system.log

# The mask rule receives the full decision event; the original input sits at
# `input.input`. `evt(override)` folds an override into the base event shape.
# Tests evaluate `log.mask` with that event, then use `has_redaction` /
# `any_patch_for_path` to assert outcomes. `with` stays inside each test.

base_event := {
	"path": "/bgz",
	"input": {
		"context": {},
		"subject": {},
		"action": {"request": {}, "fhir_rest": {}},
		"resource": {},
	},
}

evt(override) := object.union(base_event, {"input": override})

has_redaction(patches, path) if {
	{"op": "upsert", "path": path, "value": "[REDACTED]"} in patches
}

# --- patient_bsn -----------------------------------------------------------

bsn_path := "/input/context/patient_bsn"

test_redacts_patient_bsn if {
	patches := log.mask with input as evt({"context": {"patient_bsn": "123456789"}})
	has_redaction(patches, bsn_path)
}

test_no_patch_when_patient_bsn_absent if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, bsn_path)
}

test_no_patch_when_patient_bsn_empty if {
	patches := log.mask with input as evt({"context": {"patient_bsn": ""}})
	not any_patch_for_path(patches, bsn_path)
}

# --- patient_enrollment_identifier ----------------------------------------

enrollment_path := "/input/subject/patient_enrollment_identifier"

enrollment(value) := {"subject": {"patient_enrollment_identifier": value}}

test_redacts_patient_enrollment_identifier_uri_system if {
	patches := log.mask with input as evt(enrollment("http://fhir.nl/fhir/NamingSystem/bsn|123456789"))
	has_redaction(patches, enrollment_path)
}

test_redacts_patient_enrollment_identifier_oid_system if {
	patches := log.mask with input as evt(enrollment("urn:oid:2.16.840.1.113883.2.4.6.3|123456789"))
	has_redaction(patches, enrollment_path)
}

test_keeps_non_bsn_enrollment_identifier_untouched if {
	patches := log.mask with input as evt(enrollment("http://fhir.nl/fhir/NamingSystem/ura|12345"))
	not any_patch_for_path(patches, enrollment_path)
}

# --- Anomalous FHIR syntax: systemless (|<value>) and bare (<value>) -------
# The eleven-check keeps false positives low against 8-digit AGB/URA and
# 9-digit UZI numbers (UZI has no checksum).

test_redacts_systemless_bsn_with_pipe_prefix if {
	patches := log.mask with input as evt(enrollment("|900186021"))
	has_redaction(patches, enrollment_path)
}

test_redacts_bare_bsn_without_pipe if {
	patches := log.mask with input as evt(enrollment("900186021"))
	has_redaction(patches, enrollment_path)
}

test_keeps_unscoped_invalid_checksum_untouched if {
	patches := log.mask with input as evt(enrollment("|123456789"))
	not any_patch_for_path(patches, enrollment_path)
}

# UZI-zorgverlener has the same length as a BSN but no eleven-check; a
# UZI-shaped number that fails the check must be left visible in logs.
test_keeps_uzi_shaped_non_elfproef_value_untouched if {
	patches := log.mask with input as evt(enrollment("123456780"))
	not any_patch_for_path(patches, enrollment_path)
}

test_keeps_unscoped_eight_digit_value_untouched if {
	patches := log.mask with input as evt(enrollment("|12345678"))
	not any_patch_for_path(patches, enrollment_path)
}

test_keeps_empty_pipe_only_untouched if {
	patches := log.mask with input as evt(enrollment("|"))
	not any_patch_for_path(patches, enrollment_path)
}

test_keeps_multi_pipe_value_untouched if {
	patches := log.mask with input as evt(enrollment("|900186021|extra"))
	not any_patch_for_path(patches, enrollment_path)
}

# BSNs are issued starting at 100000000; a leading-zero value is not a real
# citizen BSN. 012345672 passes eleven-check so we still redact: over-redact
# rather than leak.
test_redacts_leading_zero_valid_checksum if {
	patches := log.mask with input as evt(enrollment("012345672"))
	has_redaction(patches, enrollment_path)
}

# --- search_params ----------------------------------------------------------
# Every value is redacted; the keys stay unless they don't look like a search
# parameter name.

search_params_path := "/input/action/fhir_rest/search_params"

search_params(params) := {"action": {"fhir_rest": {"search_params": params}}}

has_search_params_redaction(patches, keys) if {
	some patch in patches
	patch.path == search_params_path
	patch.value == {key: "[REDACTED]" | some key in keys}
}

test_redacts_bsn_identifier_search if {
	patches := log.mask with input as evt(search_params({"identifier": [["http://fhir.nl/fhir/NamingSystem/bsn|123456789"]]}))
	has_search_params_redaction(patches, {"identifier"})
}

test_redacts_non_bsn_identifier_search if {
	patches := log.mask with input as evt(search_params({"identifier": [["http://fhir.nl/fhir/NamingSystem/ura|12345"]]}))
	has_search_params_redaction(patches, {"identifier"})
}

test_redacts_chained_and_other_search_params if {
	params := {
		"subject:Patient.identifier": [["http://fhir.nl/fhir/NamingSystem/bsn|123456789"]],
		"patient": [["Patient/123"]],
		"_lastUpdated": [["2024-01-01"]],
	}
	patches := log.mask with input as evt(search_params(params))
	has_search_params_redaction(patches, {"subject:Patient.identifier", "patient", "_lastUpdated"})
}

test_keeps_modifier_and_reverse_chain_keys if {
	params := {"identifier:of-type": [["x"]], "_has:Observation:patient:code": [["y"]]}
	patches := log.mask with input as evt(search_params(params))
	has_search_params_redaction(patches, {"identifier:of-type", "_has:Observation:patient:code"})
}

test_redacts_search_param_keys_that_need_escaping if {
	patches := log.mask with input as evt(search_params({"a/b~c d": [["123456789"]]}))
	has_search_params_redaction(patches, {"[REDACTED]"})
}

# A malformed query ("?<system>|<bsn>" or "?<bsn>") makes the BSN the key.
test_redacts_search_param_keys_holding_a_bsn if {
	params := {
		"http://fhir.nl/fhir/NamingSystem/bsn|123456782": [[""]],
		"123456782": [[""]],
		"patient": [["Patient/123"]],
	}
	patches := log.mask with input as evt(search_params(params))
	has_search_params_redaction(patches, {"[REDACTED]", "patient"})
}

test_redacts_non_string_search_param_values if {
	patches := log.mask with input as evt(search_params({"identifier": [[{"unexpected": "shape"}]]}))
	has_search_params_redaction(patches, {"identifier"})
}

test_no_search_params_patch_when_absent_or_empty if {
	not any_patch_for_path(log.mask, search_params_path) with input as base_event
	not any_patch_for_path(log.mask, search_params_path) with input as evt(search_params({}))
}

# --- Patient-linked fields --------------------------------------------------

test_redacts_patient_id if {
	patches := log.mask with input as evt({"context": {"patient_id": "123"}})
	has_redaction(patches, "/input/context/patient_id")
}

test_redacts_resource_id if {
	patches := log.mask with input as evt({"resource": {"id": "123"}})
	has_redaction(patches, "/input/resource/id")
}

test_redacts_request_path if {
	patches := log.mask with input as evt({"action": {"request": {"path": "/Patient/123"}}})
	has_redaction(patches, "/input/action/request/path")
}

test_no_patient_linked_patches_when_empty if {
	patches := log.mask with input as evt({"context": {"patient_id": ""}, "resource": {"id": ""}, "action": {"request": {"path": ""}}})
	not any_patch_for_path(patches, "/input/context/patient_id")
	not any_patch_for_path(patches, "/input/resource/id")
	not any_patch_for_path(patches, "/input/action/request/path")
}

# --- Wholesale redactions: query, body, header, resource.content ----------

test_redacts_request_query_when_present if {
	patches := log.mask with input as evt({"action": {"request": {"query": "identifier=whatever"}}})
	has_redaction(patches, "/input/action/request/query")
}

test_no_query_patch_when_query_empty if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/query")
}

test_redacts_request_body_when_present if {
	body := `{"identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"123456789"}]}`
	patches := log.mask with input as evt({"action": {"request": {"body": body}}})
	has_redaction(patches, "/input/action/request/body")
}

test_no_body_patch_when_body_empty if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/body")
}

test_redacts_request_header_when_present if {
	header := {"Authorization": ["Bearer x"]}
	patches := log.mask with input as evt({"action": {"request": {"header": header}}})
	has_redaction(patches, "/input/action/request/header")
}

test_no_header_patch_when_header_absent if {
	patches := log.mask with input as base_event
	not any_patch_for_path(patches, "/input/action/request/header")
}

test_redacts_resource_content_when_present if {
	content := {
		"resourceType": "Patient",
		"identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "123456789"}],
	}
	patches := log.mask with input as evt({"resource": {"content": content}})
	has_redaction(patches, "/input/resource/content")
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
	patches := log.mask with input as evt(enrollment({"unexpected": "shape"}))
	not any_patch_for_path(patches, enrollment_path)
}

# URN OID is case-insensitive per RFC 8141; a system URI uppercased should
# still be detected as BSN-scoped.
test_case_insensitive_oid_match if {
	patches := log.mask with input as evt(enrollment("URN:OID:2.16.840.1.113883.2.4.6.3|123456789"))
	has_redaction(patches, enrollment_path)
}

test_case_insensitive_uri_match if {
	patches := log.mask with input as evt(enrollment("HTTP://FHIR.NL/FHIR/NAMINGSYSTEM/BSN|123456789"))
	has_redaction(patches, enrollment_path)
}

# Empty header map should not emit a pointless patch.
test_empty_header_map_produces_no_patch if {
	patches := log.mask with input as evt({"action": {"request": {"header": {}}}})
	not any_patch_for_path(patches, "/input/action/request/header")
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
