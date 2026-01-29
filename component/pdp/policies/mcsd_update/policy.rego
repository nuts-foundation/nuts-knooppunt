package mcsd_update

import rego.v1

default allow := false 

allow if {
    input.context.capability_allowed == true
}
