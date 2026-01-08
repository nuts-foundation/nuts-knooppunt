package eoverdracht.sender

required_client_qualification := "eoverdracht-sender"

default allow = false

allow if {
    allowed_by_capabilitystatement
    client_is_qualified
    user_is_authenticated_if_required
    has_consent
}

allowed_by_capabilitystatement if {
    input.capabilitystatement.checked
}

client_is_qualified if {
    required_client_qualification in input.client.qualifications
}

user_is_authenticated_if_required if {
    # No user authentication required for Task resources (contains no medical data/PII)
    input.resource.type == "Task"
}

user_is_authenticated_if_required if {
    input.resource.type != "Task"
    input.subject.properties.subject_id
}

has_consent if {
    consent := input.context.consents[_]
    # Check if consent actor FHIR identifier matches subject organization ID (FHIR token)
    actorIdentifier := consent.provision.actor.identifier[_]
    subjectOrganizationIDParts := split(input.subject.properties.subject_organization_id, "|")
    actorIdentifier.system == subjectOrganizationIDParts[0]
    actorIdentifier.value == subjectOrganizationIDParts[1]
    # Check if consent data reference matches the requested resource
    dataReference := consent.provision.data[_]
    expectedReference := sprintf("%s/%s", [input.resource.type, input.resource.properties.id])
    dataReference.reference.reference == expectedReference
}

