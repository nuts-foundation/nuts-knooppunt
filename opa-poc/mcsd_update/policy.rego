package mcsd_update

required_client_qualification := "mcsd_update_client"

default allow = false

allow if {
    allowed_by_capabilitystatement
    client_is_qualified
}

allowed_by_capabilitystatement if {
    input.capabilitystatement.checked
}

client_is_qualified if {
    required_client_qualification in input.client.qualifications
}