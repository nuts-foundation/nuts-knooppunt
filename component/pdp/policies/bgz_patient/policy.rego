package bgz_patient

import rego.v1

default allow := false

# Allow if the HTTP request matches the exact pattern:
# Patient?_include=Patient%3Ageneral-practitioner
allow if {
	input.resource.type == "Patient"
	input.action.properties.include = ["Patient:general-practitioner"]
	input.action.properties.interaction_type = "search-type"

	mitz.has_consent(input)
}

