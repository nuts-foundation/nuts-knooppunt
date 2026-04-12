package test_search_params

import rego.v1

# This policy exists solely to test AND/OR search parameter handling.
# It intentionally uses minimal allow conditions (no mitz consent, no patient check)
# so the tests can focus purely on search_params structure.

default allow := false
allow if {
    is_allowed_query
}

# GET [base]/Observation?category=a,b
# OR: category must be a or b (comma-separated in one param)
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == [["a", "b"]]
}

# GET [base]/Observation?category=1&category=2
# AND: both category=1 and category=2 must be present
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == [["1"], ["2"]]
}

# GET [base]/Observation?category=a,b&category=1
# AND of ORs: (category is a or b) AND (category is 1)
is_allowed_query if {
    input.resource.type == "Observation"
    input.action.fhir_rest.interaction_type == "search-type"
    input.action.fhir_rest.search_params.category == [["a", "b"], ["1"]]
}
