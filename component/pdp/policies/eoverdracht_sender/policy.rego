package eoverdracht_sender

import rego.v1

default allow := false
allow if {
    is_read_interaction
    has_local_consent
}

allow if {
    is_task_resource
    is_update_interaction
}

is_read_interaction if {
    input.action.fhir_rest.interaction_type == "read"
}

has_local_consent if {
    some consent in input.resource.consents
    consent.scope == "eoverdracht"
}

is_task_resource if {
    input.resource.type == "Task"
    input.resource.id != ""
}

is_update_interaction if {
    input.action.request.method == "PUT"
    input.action.fhir_rest.interaction_type == "update"
}
