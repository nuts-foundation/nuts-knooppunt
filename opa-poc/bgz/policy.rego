package bgz

default allow = false

allow {
    allowed_by_capabilitystatement
    client_is_qualified
    practitioner_is_authorized
    has_consent
}

allowed_by_capabilitystatement {
    input.capabilitystatement.checked
}

client_is_qualified {
    input.client.qualifications["bgz-requester"]
}

practitioner_is_authorized {
    input.subject.properties.subject_role == "arts"
}

has_consent {
    some consent
    consent := input.consent[_]
    # Check if consent actor FHIR identifier matches subject organization ID (FHIR token)
    some actorIdentifier
    actorIdentifier := consent.provision.actor.identifier[_]
    subjectOrganizationIDParts := {x | x := split(input.subject.properties.subject_organization_id, "|")[_]}
    actorIdentifier.system == subjectOrganizationIDParts[0]
    actorIdentifier.value == subjectOrganizationIDParts[1]
}
