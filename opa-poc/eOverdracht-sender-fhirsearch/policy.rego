package eoverdracht.sender

import data.common
import data.fhir

required_client_qualification := "eoverdracht-sender"

default allow = false

allow if {
	fhir.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
	common.user_is_authenticated_if_required
	resource_category_allowed
}

resource_category_allowed if {
	input.resource.type == "Observation"
	fhir.filter_authz({"code": "http://example.org/fhir/CodeSystem/observation-codes|heart-measurement"})
}

resource_category_allowed if {
	input.resource.type == "Condition"
	fhir.filter_authz({"code": "http://example.org/fhir/CodeSystem/condition-codes|heartfailure"})
}

deny_reason := "operation not allowed by FHIR CapabilityStatement" if {
	not allow
	not fhir.allowed_by_capabilitystatement
}

deny_reason := "client is not qualified" if {
	not allow
	not common.client_has_qualification(required_client_qualification)
}

deny_reason := "no authenticated user" if {
	not allow
	not common.user_is_authenticated_if_required
}

deny_reason := "missing patient consent" if {
	not allow
	not common.has_consent_for_requested_resource
}

deny_reason := "resource category not allowed" if {
	not allow
	not resource_category_allowed
}
