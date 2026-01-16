package mcsd_update

import data.common
import data.fhir

# Configuration
required_client_qualification := "mcsd-updater"

# Policy decision
default allow = false

allow if {
	fhir.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
}

deny_reason := "operation not allowed by FHIR CapabilityStatement" if {
	not allow
	not fhir.allowed_by_capabilitystatement
}

deny_reason := "client is not qualified" if {
	not allow
	not common.client_has_qualification(required_client_qualification)
}
