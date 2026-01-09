package eoverdracht.receiver

import data.common

# Configuration
required_client_qualification := "eoverdracht-receiver"
expected_http_method := "POST"

# Policy decision
default allow = false

allow if {
	common.http_method_is(expected_http_method)
	common.client_has_qualification(required_client_qualification)
}

deny_reason := "invalid HTTP method" if {
	not allow
	not common.http_method_is(expected_http_method)
}

deny_reason := "client is not qualified" if {
	not allow
	not common.client_has_qualification(required_client_qualification)
}
