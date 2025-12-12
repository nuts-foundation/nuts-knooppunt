package bgz

required_client_qualification := "bgz-requester"
required_subject_role := "arts"

default allow = false

allow if {
    allowed_by_capabilitystatement
    client_is_qualified
    practitioner_is_authorized
    has_consent
}

allowed_by_capabilitystatement if {
    input.capabilitystatement.checked
}

client_is_qualified if {
    required_client_qualification in input.client.qualifications
}

practitioner_is_authorized if {
    input.subject.properties.subject_role == required_subject_role
}

has_consent if {
    consent := input.consent[_]
    # Check if consent actor FHIR identifier matches subject organization ID (FHIR token)
    actorIdentifier := consent.provision.actor.identifier[_]
    subjectOrganizationIDParts := split(input.subject.properties.subject_organization_id, "|")
    actorIdentifier.system == subjectOrganizationIDParts[0]
    actorIdentifier.value == subjectOrganizationIDParts[1]
}
