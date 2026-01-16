package eoverdracht.receiver.derived

import data.common
import data.fhir

# Configuration
required_client_qualification := "eoverdracht-receiver"

# Policy decision
default allow = false

allow if {
	fhir.allowed_by_capabilitystatement
	common.client_has_qualification(required_client_qualification)
	common.user_is_authenticated_if_required

    # TODO: This only supports FHIR 'read' operations of resources other than Task and Composition.
	resource_authorized_via_task
}

resource_authorized_via_task if {
    compositions := fhir.find(
        input.context.fhir_base_url,
        "Composition",
        {"entry": input.resource.type + "/" + input.resource.properties.id}
    )
    # Find all Tasks that reference these Compositions
    some composition in compositions {
        tasks := fhir.find(
            input.context.fhir_base_url,
            "Task",
            {"input-value-reference": "Composition/" + composition.properties.id,
             "code": "http://nictiz.nl/fhir/NamingSystem/eoverdracht-task-type|eoverdracht"}
        )


    # Parse the requester's organization identifier
    org_identifier := fhir.parse_identifier(input.subject.properties.subject_organization_id)
	# Find a Task where the requester/owner matches the current organization
	task := find_task_for_organization(org_identifier.system, org_identifier.value)

	# Get the Composition reference from the Task's input
	composition_ref := get_composition_from_task(task)

	# Parse the Composition reference (format: "Composition/id")
	composition_parts := split(composition_ref, "/")
	count(composition_parts) == 2
	composition_id := composition_parts[1]

	# Fetch the Composition
	composition := fhir.fetch_resource(
		input.context.fhir_base_url,
		"Composition",
		composition_id
	)

	# Check if the requested resource is referenced by the Composition
	resource_referenced_in_composition(composition, input.resource.type, input.resource.properties.id)
}

# Helper: Find a Task where the organization is the owner or requester
find_task_for_organization(system, value) := task if {
	# Search for Tasks with eOverdracht type
	search_url := sprintf("%s/Task?code=%s", [
		input.context.fhir_base_url,
		"http://nictiz.nl/fhir/NamingSystem/eoverdracht-task-type|eoverdracht"
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
	response.body.entry
	count(response.body.entry) > 0

	# Find a task where the organization is a participant (owner or requester)
	task := response.body.entry[_].resource
	fhir.is_task_participant(task, system, value)
}

# Helper: Extract the Composition reference from a Task's input
get_composition_from_task(task) := reference if {
	# Task.input contains the reference to the Composition
	input_item := task.input[_]
	input_item.type.coding[_].code == "composition"
	reference := input_item.valueReference.reference
}

# Helper: Check if a resource is referenced in the Composition
resource_referenced_in_composition(composition, resource_type, resource_id) if {
	# Build the expected reference string
	expected_ref := sprintf("%s/%s", [resource_type, resource_id])

	# Check if it's in the composition's section entries
	section := composition.section[_]
	entry := section.entry[_]
	entry.reference == expected_ref
}

# Alternative: Check direct references in top-level composition entries
resource_referenced_in_composition(composition, resource_type, resource_id) if {
	expected_ref := sprintf("%s/%s", [resource_type, resource_id])
	entry := composition.entry[_]
	entry.reference == expected_ref
}

# Deny reasons for better error messages
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

deny_reason := "resource not authorized via task" if {
	not allow
	not resource_authorized_via_task
}

