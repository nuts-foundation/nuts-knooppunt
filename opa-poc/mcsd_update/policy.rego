package mcsd_update

import data.common

# Configuration
required_client_qualification := "mcsd_update_client"

# Policy decision
default allow = false

allow if {
	common.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
}

deny_reason := "operation not allowed by FHIR CapabilityStatement" if {
	not allow
	not common.allowed_by_capabilitystatement
}

deny_reason := "client is not qualified" if {
	not allow
	not common.client_has_qualification(required_client_qualification)
}
