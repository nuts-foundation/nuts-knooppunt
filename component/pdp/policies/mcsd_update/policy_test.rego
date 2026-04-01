package mcsd_update_test

import rego.v1

import data.mcsd_update

test_allow_capability_checked if {
	mcsd_update.allow with input as {"action": {"fhir_rest": {"capability_checked": true}}}
}

test_deny_capability_not_checked if {
	not mcsd_update.allow with input as {"action": {"fhir_rest": {"capability_checked": false}}}
}

test_deny_capability_missing if {
	not mcsd_update.allow with input as {"action": {"fhir_rest": {}}}
}
