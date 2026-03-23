package eoverdracht_notification

import rego.v1

default allow := false
allow if {
    input.subject.organization.ura != ""
    input.action.request.method == "POST"
    input.action.request.path = "/Task"
}
