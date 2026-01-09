package fhir

# ===================================================
# FHIR Helper Functions
# ===================================================
# This file contains reusable FHIR-related helper functions
# that can be imported by specific use-case policies.

# ---------------------------------------------------
# CapabilityStatement Validation
# ---------------------------------------------------

# Check if the CapabilityStatement has been verified
allowed_by_capabilitystatement if {
    input.capabilitystatement.checked
}

# ---------------------------------------------------
# FHIR Search Authorization
# ---------------------------------------------------

# Helper: Perform FHIR search with filters to authorize access
# Returns true if the resource matches the filter criteria
# Parameters is a map/dictionary of FHIR search parameters (e.g., {"code": "value", "category": "vital-signs"})
filter_authz(parameters) if {
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

