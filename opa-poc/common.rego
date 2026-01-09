package common

# ===================================================
# Common Policy Components
# ===================================================
# This file contains reusable policy components that can be
# imported by specific use-case policies.

# ---------------------------------------------------
# CapabilityStatement Validation
# ---------------------------------------------------

# Check if the CapabilityStatement has been verified
allowed_by_capabilitystatement if {
    input.capabilitystatement.checked
}

# ---------------------------------------------------
# Client Qualification Checks
# ---------------------------------------------------

# Check if client has ALL required qualifications from a set
client_has_qualifications(required_qualifications) if {
    every qual in required_qualifications {
        qual in input.client.qualifications
    }
}

# Check if client has a specific qualification
client_has_qualification(required_qualification) if {
    required_qualification in input.client.qualifications
}

# ---------------------------------------------------
# Consent Validation
# ---------------------------------------------------

# Check if there's a consent matching the subject organization
# The consent actor's FHIR identifier must match the subject organization ID
has_consent_for_subject_organization if {
    consent := input.context.consents[_]
    _consent_actor_matches_subject_org(consent)
}

# Check if there's a consent for the requested resource (from input)
has_consent_for_requested_resource if {
    consent := input.context.consents[_]
    _consent_actor_matches_subject_org(consent)
    _consent_data_matches_resource(consent, input.resource.type, input.resource.properties.id)
}

# Helper: Check if consent actor matches subject organization
_consent_actor_matches_subject_org(consent) if {
    actorIdentifier := consent.provision.actor.identifier[_]
    subjectOrganizationIDParts := split(input.subject.properties.subject_organization_id, "|")
    actorIdentifier.system == subjectOrganizationIDParts[0]
    actorIdentifier.value == subjectOrganizationIDParts[1]
}

# Helper: Check if consent data reference matches a specific resource
_consent_data_matches_resource(consent, resource_type, resource_id) if {
    dataReference := consent.provision.data[_]
    expectedReference := sprintf("%s/%s", [resource_type, resource_id])
    dataReference.reference.reference == expectedReference
}

# Alternative consent check for simpler structure (bgz style)
has_consent_for_subject_organization_simple if {
    consent := input.consent[_]
    actorIdentifier := consent.provision.actor.identifier[_]
    subjectOrganizationIDParts := split(input.subject.properties.subject_organization_id, "|")
    actorIdentifier.system == subjectOrganizationIDParts[0]
    actorIdentifier.value == subjectOrganizationIDParts[1]
}

# ---------------------------------------------------
# User Authentication
# ---------------------------------------------------

# Check if user is authenticated (subject_id is present)
user_is_authenticated if {
    input.subject.properties.subject_id
}

# Check if user authentication is required based on resource type
# Task resources don't require authentication (no medical data/PII)
user_authentication_required if {
    input.resource.type != "Task"
}

# Check if user is authenticated when required
# Returns true if authentication is not required OR user is authenticated
user_is_authenticated_if_required if {
    not user_authentication_required
}

user_is_authenticated_if_required if {
    user_is_authenticated
}

# ---------------------------------------------------
# HTTP Method Validation
# ---------------------------------------------------

# Check if HTTP method matches expected value
http_method_is(expected_method) if {
    input.action.properties.method == expected_method
}

# ---------------------------------------------------
# Role-Based Authorization
# ---------------------------------------------------

# Check if subject has a specific role
subject_has_role(required_role) if {
    input.subject.properties.subject_role == required_role
}

# ---------------------------------------------------
# Denial Reason Generators
# ---------------------------------------------------

# Generate denial reason for capability statement check failure
# Usage: reason := common.denial_reason_capabilitystatement_failed
denial_reason_capabilitystatement_failed := "CapabilityStatement check failed"
denial_reason_client_not_qualified := "Client is not qualified"

# Generate denial reason for consent check failure (detailed)
# Usage: reason := common.denial_reason_no_consent_for_resource
denial_reason_no_consent_for_resource := "No matching consent found for the requested resource"

# Generate denial reason for consent check failure (simple)
# Usage: reason := common.denial_reason_no_consent_for_organization
denial_reason_no_consent_for_organization := "No matching consent found for subject organization"

# Generate denial reason for user authentication failure
# Usage: reason := common.denial_reason_user_not_authenticated
denial_reason_user_not_authenticated := "User authentication required but subject_id is missing"

# Generate denial reason for HTTP method mismatch
# Usage: reason := common.denial_reason_invalid_http_method("POST", "GET")
denial_reason_invalid_http_method(expected_method, actual_method) := reason if {
    reason := sprintf("Invalid HTTP method: expected %s, got %s", [expected_method, actual_method])
}

# Generate denial reason for role mismatch
# Usage: reason := common.denial_reason_invalid_role("arts")
denial_reason_invalid_role(required_role) := reason if {
    reason := sprintf("Subject does not have required role (required: %s)", [required_role])
}




