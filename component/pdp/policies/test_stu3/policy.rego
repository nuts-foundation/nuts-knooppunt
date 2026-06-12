package test_stu3

import rego.v1

# This policy exists solely to test STU3 capability statement parsing.

default allow := false
allow if {
	input.action.fhir_rest.capability_checked == true
}
