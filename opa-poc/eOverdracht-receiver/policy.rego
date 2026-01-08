package eoverdracht.receiver

# Required qualification for the client to receive eOverdracht data
required_client_qualification := "eoverdracht-receiver"

default allow = false

allow if {
    is_post_request
    client_is_qualified
}

# Verify that the HTTP method is POST
is_post_request if {
    input.action.properties.method == "POST"
}

# Verify that the client has the required qualification
client_is_qualified if {
    required_client_qualification in input.client.qualifications
}

