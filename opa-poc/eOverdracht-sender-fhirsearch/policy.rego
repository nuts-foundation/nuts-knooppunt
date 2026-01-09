package eoverdracht.sender

import data.common

required_client_qualification := "eoverdracht-sender"

default allow = false

allow if {
	common.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
	common.user_is_authenticated_if_required
	common.has_consent_for_requested_resource
	resource_category_allowed
}

resource_category_allowed if {
	input.resource.type == "Observation"
	# Fetch the resource and check its code
	resource := _fetch_fhir_resource(input.resource.type, input.resource.properties.id)
	_resource_has_code(resource, "http://example.org/fhir/CodeSystem/observation-codes", "heart-measurement")
}

resource_category_allowed if {
	input.resource.type == "Condition"
	# Fetch the resource and check its code
	resource := _fetch_fhir_resource(input.resource.type, input.resource.properties.id)
	_resource_has_code(resource, "http://example.org/fhir/CodeSystem/condition-codes", "heartfailure")
}

# Helper: Fetch a FHIR resource by type and ID
_fetch_fhir_resource(resource_type, resource_id) := response.body if {
	url := sprintf("%s/%s/%s", [input.context.fhir_base_url, resource_type, resource_id])

	response := http.send({
		"method": "GET",
		"url": url,
		"headers": {
			"Accept": "application/fhir+json"
		},
		"raise_error": false
	})

	response.status_code == 200
}

# Helper: Check if resource has a specific code
_resource_has_code(resource, system, code) if {
	coding := resource.code.coding[_]
	coding.system == system
	coding.code == code
}

# Helper: Perform FHIR search with filters to authorize access
# Returns true if the resource matches the filter criteria
# Parameters is a map/dictionary of FHIR search parameters (e.g., {"code": "value", "category": "vital-signs"})
fhir_filter_authz(parameters) if {
	count(parameters) > 0

	# Build query string from parameters map
	query_params := [param_string |
		some key, value in parameters
		param_string := sprintf("%s=%s", [key, value])
	]
	query_string := concat("&", query_params)

	search_url := sprintf("%s/%s?_summary=count&_id=%s&%s", [
		input.context.fhir_base_url,
		input.resource.type,
		input.resource.properties.id,
		query_string
	])

	response := http.send({
		"method": "GET",
		"url": search_url,
		"headers": {
			"Accept": "application/fhir+json"
		},
		"raise_error": false
	})

	# Allow if the search returns results (status 200 and total > 0)
	response.status_code == 200
	response.body.total > 0
}

deny_reason := "operation not allowed by FHIR CapabilityStatement" if {
	not allow
	not common.allowed_by_capabilitystatement
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
