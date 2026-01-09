package bgz

import data.common
import data.fhir

# Configuration
required_client_qualification := "bgz-requester"
required_subject_role := "arts"

# Policy decision
default allow = false

allow if {
	fhir.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
	common.subject_has_role(required_subject_role)
	common.has_consent_for_subject_organization_simple
}

deny_reason := "operation not allowed by FHIR CapabilityStatement" if {
	not allow
	not fhir.allowed_by_capabilitystatement
}

deny_reason := "client is not qualified" if {
	not allow
	not common.client_has_qualification(required_client_qualification)
}

deny_reason := "subject does not have required role" if {
	not allow
	not common.subject_has_role(required_subject_role)
}

deny_reason := "missing patient consent" if {
	not allow
	not common.has_consent_for_subject_organization_simple
}
