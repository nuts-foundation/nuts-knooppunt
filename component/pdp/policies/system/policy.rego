# The directory is flat (`system/`) because the bundler in
# component/pdp/policies/bundler.go requires `{policyName}/policy.rego` at the
# top level of each policy directory; nesting as `system/log/` would break
# bundle generation.
# regal ignore:directory-package-mismatch
package system.log

import rego.v1

# mask redacts BSNs (Dutch social security numbers) from OPA decision logs.
# OPA calls this rule for every decision event with the event as `input`, so
# the original decision input lives at `input.input`. The rule returns
# JSON-Patch operations describing what to redact.
# See https://www.openpolicyagent.org/docs/management-decision-logs#masking-sensitive-data

redacted := "[REDACTED]"

# FHIR NamingSystem URI and equivalent HL7 OID that identify a BSN. Identifiers
# scoped to other systems (URA, AGB, etc.) are left untouched.
bsn_system_uri := "http://fhir.nl/fhir/NamingSystem/bsn"

bsn_system_oid := "urn:oid:2.16.840.1.113883.2.4.6.3"

# --- Scalar redactions: known BSN-bearing fields ----------------------------

mask contains {"op": "upsert", "path": "/input/context/patient_bsn", "value": redacted} if {
	input.input.context.patient_bsn != ""
}

mask contains {"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": redacted} if {
	is_bsn_scoped(input.input.subject.patient_enrollment_identifier)
}

# --- Structured redactions: search params ----------------------------------
# search_params.identifier is a [][]string; when any value is BSN-scoped we
# replace the whole key with a placeholder (rather than removing it) so the
# log still shows the field was present and deliberately redacted.

mask contains {"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": redacted} if {
	some and_group in input.input.action.fhir_rest.search_params.identifier
	some value in and_group
	is_bsn_scoped(value)
}

# --- Wholesale redactions: debug-only fields that can embed BSNs -----------
# These fields are not read by any policy; they exist in the decision input
# only for traceability. Redacting specific BSN shapes inside them (JSON
# bodies, URL-encoded identifiers, chained FHIR search params, etc.) is an
# endless arms race, so we replace the whole value with a placeholder. The
# key stays so readers can see the field existed but was deliberately
# redacted — and a future use case could swap to selective masking without
# restructuring the decision-log event shape.

mask contains {"op": "upsert", "path": "/input/action/request/query", "value": redacted} if {
	input.input.action.request.query != ""
}

mask contains {"op": "upsert", "path": "/input/action/request/body", "value": redacted} if {
	input.input.action.request.body != ""
}

mask contains {"op": "upsert", "path": "/input/action/request/header", "value": redacted} if {
	count(input.input.action.request.header) > 0
}

mask contains {"op": "upsert", "path": "/input/resource/content", "value": redacted} if {
	input.input.resource.content
}

# --- Helpers ---------------------------------------------------------------
# `is_string` guards against fail-open: without it, a non-string value would
# raise a type error from startswith, the rule body would become undefined
# and OPA would log the decision event unmasked.
# URN comparison is case-insensitive per RFC 8141; FHIR URIs are case-
# sensitive per spec, but accepting case-insensitive matching here is a
# defensive choice for a log-scrubber.

is_bsn_scoped(s) if {
	is_string(s)
	startswith(lower(s), sprintf("%s|", [lower(bsn_system_uri)]))
}

is_bsn_scoped(s) if {
	is_string(s)
	startswith(lower(s), sprintf("%s|", [lower(bsn_system_oid)]))
}
