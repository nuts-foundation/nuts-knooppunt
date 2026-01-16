package eoverdracht.sender

import data.common
import data.fhir

required_client_qualification := "eoverdracht-sender"

default allow = false

allow if {
	fhir.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
	common.user_is_authenticated_if_required
	has_eoverdracht_task
	resource_category_allowed
}

# Check if there's an eOverdracht Task where the client is the requester or owner
has_eoverdracht_task if {
	# Parse subject_organization_id (format: "system|value")
	org_identifier := fhir.parse_identifier(input.subject.properties.subject_organization_id)

	# Fetch the resource being accessed to get its patient reference
	resource := fhir.fetch_resource(
		input.context.fhir_base_url,
		input.resource.type,
		input.resource.properties.id
	)

	# Extract patient reference based on resource type
	patient_ref := fhir.patient_reference(input.resource.type, resource)

	# Check if Task exists for this patient with eOverdracht code
	has_task(
		patient_ref,
		"http://nictiz.nl/fhir/NamingSystem/eoverdracht-task-type|eoverdracht",
		org_identifier.system,
		org_identifier.value
	)
}


# Generic helper: Check if a FHIR Task exists for given patient, code, and requester
# Parameters:
#   - patient: Patient reference (e.g., "Patient/123")
#   - task_code: Task code in system|code format
#   - requester_system: Organization identifier system
#   - requester_value: Organization identifier value
has_task(patient, task_code, requester_system, requester_value) if {
	# Search for Tasks by patient and code
	search_url := sprintf("%s/Task?patient=%s&code=%s", [
		input.context.fhir_base_url,
		patient,
		task_code
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

	response.status_code == 200

	# Ensure entry array exists and is not empty
	response.body.entry
	count(response.body.entry) > 0

	# Get a task from the response
	task := response.body.entry[_].resource

	# Filter: Check if requester or owner matches the organization
	fhir.is_task_participant(task, requester_system, requester_value)
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
	not fhir.allowed_by_capabilitystatement
} else := "client is not qualified" if {
	not common.client_has_qualification(required_client_qualification)
} else := "no authenticated user" if {
	not common.user_is_authenticated_if_required
} else := "no eOverdracht task for organization" if {
	not has_eoverdracht_task
} else := "resource category not allowed" if {
	not resource_category_allowed
}
