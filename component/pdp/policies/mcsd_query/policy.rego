package mcsd_query

import rego.v1

default allow := true

allow if {
    input.action.fhir_rest.capability_checked == true
}
