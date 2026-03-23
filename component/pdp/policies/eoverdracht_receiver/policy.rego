package eoverdracht_receiver

import rego.v1

default allow := false
allow if {
    input.action.fhir_rest.interaction_type == "read"

    some consent in input.resource.consents
    consent.scope == "eoverdracht"
}

allow if {
    input.action.request.method == "PUT"
    input.action.fhir_rest.interaction_type == "update"
    input.resource.type == "Task"
    input.resource.id != ""
}
