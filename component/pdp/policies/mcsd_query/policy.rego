package mcsd_query

import rego.v1

default allow := false
allow if {
    request_conforms_fhir_capabilitystatement
}

default request_conforms_fhir_capabilitystatement := false
request_conforms_fhir_capabilitystatement if {
    input.action.fhir_rest.capability_checked == true
}