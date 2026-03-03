package mcsd_query

import rego.v1
import data.fhir

default allow := false
allow if {
    request_conforms_fhir_capabilitystatement
}

default request_conforms_fhir_capabilitystatement := false
request_conforms_fhir_capabilitystatement if {
    fhir.capability_statement_allowed(input.capability_statement, input.resource.type, input.action.fhir_rest)
}

