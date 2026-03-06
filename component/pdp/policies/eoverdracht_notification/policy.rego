package eoverdracht_notification

import rego.v1

default allow := false
allow if {
    input.subject.organization_ura != ""
    input.action.request.method == "POST"
}
