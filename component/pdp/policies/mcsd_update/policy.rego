package mcsd_update

import rego.v1

default allow := false 

allow if {
    input.action.fhir_rest.capability_checked == true
}
