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
# FHIR Identifier Parsing
# ---------------------------------------------------

# Helper: Parse FHIR identifier in system|value format
# Parameters:
#   - identifier_string: Identifier in "system|value" format
# Returns: Object with system and value fields
parse_identifier(identifier_string) := {"system": parts[0], "value": parts[1]} if {
	parts := split(identifier_string, "|")
	count(parts) == 2
}

# ---------------------------------------------------
# FHIR Task Helpers
# ---------------------------------------------------

# Helper: Check if Task requester matches the organization identifier
is_task_participant(task, system, value) if {
	task.requester.identifier.system == system
	task.requester.identifier.value == value
}

# Helper: Check if Task owner matches the organization identifier
is_task_participant(task, system, value) if {
	task.owner.identifier.system == system
	task.owner.identifier.value == value
}

# ---------------------------------------------------
# FHIR Resource Patient Reference Extraction
# ---------------------------------------------------

# Helper: Get patient reference from a FHIR resource based on resource type
# Most clinical resources use 'subject', but some like Task use 'for'
# Parameters:
#   - resource_type: The FHIR resource type (e.g., "Observation", "Task")
#   - resource: The FHIR resource body
patient_reference(resource_type, resource) := resource.subject.reference if {
	# Resources that use 'subject' field
	resource_type in ["Observation", "Condition", "DiagnosticReport", "Procedure", "MedicationStatement", "AllergyIntolerance", "CarePlan", "ClinicalImpression", "Encounter"]
}

patient_reference(resource_type, resource) := resource.for.reference if {
	# Resources that use 'for' field
	resource_type in ["Task"]
}

patient_reference(resource_type, resource) := resource.patient.reference if {
	# Resources that use 'patient' field
	resource_type in ["Appointment", "AppointmentResponse"]
}

# ---------------------------------------------------
# FHIR Resource Fetching
# ---------------------------------------------------

# Helper: Fetch a FHIR resource by type and ID
# Parameters:
#   - base_url: FHIR base URL
#   - resource_type: Resource type (e.g., "Observation", "Condition")
#   - resource_id: Resource ID
# Returns: The resource body if successful, undefined otherwise
fetch_resource(base_url, resource_type, resource_id) := response.body if {
	resource_url := sprintf("%s/%s/%s", [base_url, resource_type, resource_id])

	response := http.send({
		"method": "GET",
		"url": resource_url,
		"headers": {
			"Accept": "application/fhir+json"
		},
		"raise_error": false,
		"timeout": "5s"
	})

	response.status_code == 200
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
		"raise_error": false,
		"timeout": "5s"
	})

	# Allow if the search returns results (status 200 and total > 0)
	response.status_code == 200
	response.body.total > 0
}

