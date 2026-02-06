package eoverdracht_notification

import rego.v1

default allow := false
allow if {
    input.subject.properties.subject_organization_id != ""
    input.action.properties.request.method == "POST"
}
