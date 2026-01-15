package bgz_patient

import rego.v1

default allow := false

# Allow if the HTTP request matches the exact pattern:
# Patient?_include=Patient%3Ageneral-practitioner
allow if {
	input.resource.type == "Patient"
	input.request.query_params == {"_include": ["Patient:general-practitioner"]}
}

