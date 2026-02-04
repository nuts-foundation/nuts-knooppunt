package mcsd_query

import rego.v1

default allow := true

allow if {
    input.action.properties.connection_data.fhir_rest.capability_checked == true
}
