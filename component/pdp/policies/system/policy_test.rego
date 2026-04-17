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

# --- search_params.identifier ---------------------------------------------

search_identifier_path := "/input/action/fhir_rest/search_params/identifier"

search_params(values) := {"action": {"fhir_rest": {"search_params": {"identifier": values}}}}

test_redacts_search_params_identifier_uri_system if {
	patches := log.mask with input as evt(search_params([["http://fhir.nl/fhir/NamingSystem/bsn|123456789"]]))
	has_redaction(patches, search_identifier_path)
}

test_redacts_search_params_identifier_oid_system if {
	patches := log.mask with input as evt(search_params([["urn:oid:2.16.840.1.113883.2.4.6.3|123456789"]]))
	has_redaction(patches, search_identifier_path)
}

test_redacts_identifier_when_bsn_is_second_or_value if {
	values := [[
		"http://fhir.nl/fhir/NamingSystem/ura|12345",
		"http://fhir.nl/fhir/NamingSystem/bsn|123456789",
	]]
	patches := log.mask with input as evt(search_params(values))
	has_redaction(patches, search_identifier_path)
}

test_keeps_non_bsn_identifier_search_untouched if {
	patches := log.mask with input as evt(search_params([["http://fhir.nl/fhir/NamingSystem/ura|12345"]]))
	not any_patch_for_path(patches, search_identifier_path)
}

test_keeps_other_search_params_untouched if {
	other := {"action": {"fhir_rest": {"search_params": {"_lastUpdated": [["2024-01-01"]]}}}}
	patches := log.mask with input as evt(other)
	not any_patch_for_path(patches, search_identifier_path)
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

test_non_string_search_params_identifier_does_not_break_mask if {
	patches := log.mask with input as evt(search_params([[{"unexpected": "shape"}]]))
	not any_patch_for_path(patches, search_identifier_path)
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
