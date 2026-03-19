package eoverdracht_receiver

import rego.v1

default allow := false
allow if {
    some consent in input.resource.consents
    consent.scope == "eoverdracht"
}
