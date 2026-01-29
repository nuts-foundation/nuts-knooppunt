package mcsd_update

import rego.v1

default allow := false 

allow if {
    input.context.fhir_capability_checked == true
}
