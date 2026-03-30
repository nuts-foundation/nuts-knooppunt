package mcsd_query_test

import rego.v1

import data.mcsd_query

test_allow_capability_checked if {
	mcsd_query.allow with input as {"action": {"fhir_rest": {"capability_checked": true}}}
}

test_deny_capability_not_checked if {
	not mcsd_query.allow with input as {"action": {"fhir_rest": {"capability_checked": false}}}
}

test_deny_capability_missing if {
	not mcsd_query.allow with input as {"action": {"fhir_rest": {}}}
}
