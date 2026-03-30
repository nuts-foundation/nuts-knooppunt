package eoverdracht_sender_test

import rego.v1

import data.eoverdracht_sender

base_read_input := {
	"action": {"fhir_rest": {"interaction_type": "read"}},
	"resource": {"consents": [{"scope": "eoverdracht"}]},
}

base_update_input := {
	"action": {
		"request": {"method": "PUT"},
		"fhir_rest": {"interaction_type": "update"},
	},
	"resource": {
		"type": "Task",
		"id": "task-123",
		"consents": [{"scope": "eoverdracht"}],
	},
}

base_composition_input := {
	"action": {"fhir_rest": {
		"interaction_type": "operation",
		"operation": "document",
	}},
	"resource": {
		"type": "Composition",
		"id": "comp-456",
		"consents": [{"scope": "eoverdracht"}],
	},
}

# Read path

test_allow_read_with_consent if {
	eoverdracht_sender.allow with input as base_read_input
}

test_deny_read_without_consent if {
	not eoverdracht_sender.allow with input as object.union(base_read_input, {"resource": {"consents": []}})
}

test_deny_read_wrong_consent_scope if {
	not eoverdracht_sender.allow with input as object.union(base_read_input, {"resource": {"consents": [{"scope": "other"}]}})
}

# Task update path

test_allow_task_update_with_consent if {
	eoverdracht_sender.allow with input as base_update_input
}

test_deny_task_update_without_consent if {
	not eoverdracht_sender.allow with input as object.union(base_update_input, {"resource": {"consents": []}})
}

test_deny_task_update_empty_resource_id if {
	not eoverdracht_sender.allow with input as object.union(base_update_input, {"resource": {"id": ""}})
}

test_deny_task_wrong_method if {
	not eoverdracht_sender.allow with input as object.union(base_update_input, {"action": {"request": {"method": "GET"}}})
}

# Composition $document path

test_allow_composition_document_with_consent if {
	eoverdracht_sender.allow with input as base_composition_input
}

test_deny_composition_document_without_consent if {
	not eoverdracht_sender.allow with input as object.union(base_composition_input, {"resource": {"consents": []}})
}

test_deny_composition_empty_resource_id if {
	not eoverdracht_sender.allow with input as object.union(base_composition_input, {"resource": {"id": ""}})
}

test_deny_composition_wrong_operation if {
	not eoverdracht_sender.allow with input as object.union(base_composition_input, {"action": {"fhir_rest": {"operation": "other"}}})
}

# Read path allows any resource type — only checks interaction_type + consent
test_allow_read_any_resource_type if {
	eoverdracht_sender.allow with input as object.union(base_read_input, {"resource": {
		"type": "SomeUnknownType",
		"consents": [{"scope": "eoverdracht"}],
	}})
}
