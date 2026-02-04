package mcsd_update

import rego.v1

default allow := false 

allow if {
    input.action.properties.connection_data.fhir_rest.capability_checked == true
}
