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
	is_bsn(input.input.subject.patient_enrollment_identifier)
}

mask contains {"op": "upsert", "path": "/input/subject/patient_enrollment_identifier", "value": redacted} if {
	is_potential_bsn(input.input.subject.patient_enrollment_identifier)
}

# --- Structured redactions: search params ----------------------------------
# search_params.identifier is a [][]string; when any value looks like a BSN
# we replace the whole key with a placeholder (rather than removing it) so
# the log still shows the field was present and deliberately redacted.

mask contains {"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": redacted} if {
	some and_group in input.input.action.fhir_rest.search_params.identifier
	some value in and_group
	is_bsn(value)
}

mask contains {"op": "upsert", "path": "/input/action/fhir_rest/search_params/identifier", "value": redacted} if {
	some and_group in input.input.action.fhir_rest.search_params.identifier
	some value in and_group
	is_potential_bsn(value)
}

# --- Wholesale redactions: debug-only fields that can embed BSNs -----------
# These fields are not read by any policy; they exist in the decision input
# only for traceability. Redacting specific BSN shapes inside them (JSON
# bodies, URL-encoded identifiers, chained FHIR search params, etc.) is an
# endless arms race, so we replace the whole value with a placeholder. The
# key stays so readers can see the field existed but was deliberately
# redacted, and a future use case could swap to selective masking without
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
# `is_string` guards against fail-open: without it a non-string value would
# raise a type error, the rule body would be undefined, and OPA would log
# the decision event unmasked.
# URN comparison is case-insensitive per RFC 8141; FHIR URIs are case-
# sensitive per spec, but accepting case-insensitive matching here is a
# defensive choice for a log-scrubber.

is_bsn(s) if {
	is_string(s)
	startswith(lower(s), sprintf("%s|", [lower(bsn_system_uri)]))
}

is_bsn(s) if {
	is_string(s)
	startswith(lower(s), sprintf("%s|", [lower(bsn_system_oid)]))
}

# is_potential_bsn matches an identifier value carrying a BSN without the
# system attached: either the `|<value>` form (empty system) or a bare
# `<value>`. Spec-compliant Dutch FHIR clients always include the system, so
# this only fires on anomalous input. Requiring a 9-digit number that passes
# the eleven-check keeps false positives low: UZI-zorgverlener is 9 digits
# but has no checksum, AGB and URA are 8 digits, and roughly 1 in 11 random
# 9-digit numbers happens to pass.
is_potential_bsn(s) if {
	is_string(s)
	is_valid_bsn_checksum(trim_prefix(s, "|"))
}

# Dutch BSN eleven-check: sum of 9·d1 + 8·d2 + ... + 2·d8 + (-1)·d9 must be
# divisible by 11. See https://nl.wikipedia.org/wiki/Burgerservicenummer#Elfproef
bsn_weights := [9, 8, 7, 6, 5, 4, 3, 2, -1]

is_valid_bsn_checksum(s) if {
	regex.match(`^[0-9]{9}$`, s)
	products := [p |
		i := [0, 1, 2, 3, 4, 5, 6, 7, 8][_]
		p := bsn_weights[i] * to_number(substring(s, i, 1))
	]
	sum(products) % 11 == 0
}
