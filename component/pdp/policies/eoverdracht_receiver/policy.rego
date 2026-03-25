package eoverdracht_receiver

import rego.v1

default allow := false
allow if {
    input.subject.organization.ura != ""
    input.action.request.method == "POST"
    
    some path in ["", "/"]
    input.action.request.path == path
}
