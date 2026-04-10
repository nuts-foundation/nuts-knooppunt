package bgz_test

import rego.v1

import data.bgz

base_input := {
	"action": {"fhir_rest": {
		"capability_checked": true,
		"interaction_type": "search-type",
		"include": ["Patient:general-practitioner"],
	}},
	"context": {
		"mitz_consent": true,
		"patient_id": "patient-123",
	},
	"resource": {"type": "Patient"},
}

# Precondition tests

test_deny_without_capability_checked if {
	not bgz.allow with input as object.union(base_input, {
		"action": {"fhir_rest": {"capability_checked": false}},
	})
}

test_deny_without_mitz_consent if {
	not bgz.allow with input as object.union(base_input, {"context": {"mitz_consent": false}})
}

test_deny_without_patient_identifier if {
	not bgz.allow with input as object.union(base_input, {
		"context": {"patient_id": "", "patient_bsn": ""},
	})
}

test_allow_with_patient_bsn_instead_of_id if {
	bgz.allow with input as object.union(base_input, {
		"context": {"patient_id": "", "patient_bsn": "123456789"},
	})
}

# Query 1: GET [base]/Patient?_include=Patient:general-practitioner
test_allow_patient_search if {
	bgz.allow with input as base_input
}

# Query 2: GET [base]/Coverage?_include=Coverage:payor:Patient&_include=Coverage:payor:Organization
test_allow_coverage_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Coverage"},
		"action": {"fhir_rest": {"include": ["Coverage:payor:Patient", "Coverage:payor:Organization"]}},
	})
}

# Same query with reversed include order — documents that set comparison is order-independent
test_allow_coverage_search_reversed_includes if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Coverage"},
		"action": {"fhir_rest": {"include": ["Coverage:payor:Organization", "Coverage:payor:Patient"]}},
	})
}

# Query 3: GET [base]/Consent?category=http://snomed.info/sct|11291000146105
test_allow_consent_search_category_11291 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Consent"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"category": [["http://snomed.info/sct|11291000146105"]]},
		}},
	})
}

# Query 4: GET [base]/Consent?category=http://snomed.info/sct|11341000146107
test_allow_consent_search_category_11341 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Consent"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"category": [["http://snomed.info/sct|11341000146107"]]},
		}},
	})
}

# Query 5: GET [base]/Observation/$lastn?category=http://snomed.info/sct|118228005,http://snomed.info/sct|384821006
test_allow_observation_lastn_category if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"interaction_type": "operation",
			"operation": "lastn",
			"include": [],
			"search_params": {"category": [["http://snomed.info/sct|118228005", "http://snomed.info/sct|384821006"]]},
		}},
	})
}

# Query 6: GET [base]/Condition
test_allow_condition_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Condition"},
		"action": {"fhir_rest": {"include": []}},
	})
}

# Query 7: GET [base]/Observation/$lastn?code=http://snomed.info/sct|365508006
test_allow_observation_lastn_code_365508006 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"interaction_type": "operation",
			"operation": "lastn",
			"include": [],
			"search_params": {"code": [["http://snomed.info/sct|365508006"]]},
		}},
	})
}

# Query 8: GET [base]/Observation?code=http://snomed.info/sct|228366006
test_allow_observation_search_code_228366006 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"code": [["http://snomed.info/sct|228366006"]]},
		}},
	})
}

# Query 9: GET [base]/Observation?code=http://snomed.info/sct|228273003
test_allow_observation_search_code_228273003 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"code": [["http://snomed.info/sct|228273003"]]},
		}},
	})
}

# Query 10: GET [base]/Observation?code=http://snomed.info/sct|365980008
test_allow_observation_search_code_365980008 if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"code": [["http://snomed.info/sct|365980008"]]},
		}},
	})
}

# Query 11: GET [base]/NutritionOrder
test_allow_nutrition_order_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "NutritionOrder"},
		"action": {"fhir_rest": {"include": []}},
	})
}

# Query 12: GET [base]/Flag
test_allow_flag_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Flag"},
		"action": {"fhir_rest": {"include": []}},
	})
}

# Query 13: GET [base]/AllergyIntolerance
test_allow_allergy_intolerance_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "AllergyIntolerance"},
		"action": {"fhir_rest": {"include": []}},
	})
}

# Query 14: GET [base]/MedicationStatement?category=...|6&_include=MedicationStatement:medication
test_allow_medication_statement_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "MedicationStatement"},
		"action": {"fhir_rest": {
			"include": ["MedicationStatement:medication"],
			"search_params": {"category": [["urn:oid:2.16.840.1.113883.2.4.3.11.60.20.77.5.3|6"]]},
		}},
	})
}

# Query 15: GET [base]/MedicationRequest?category=http://snomed.info/sct|16076005&_include=MedicationRequest:medication
test_allow_medication_request_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "MedicationRequest"},
		"action": {"fhir_rest": {
			"include": ["MedicationRequest:medication"],
			"search_params": {"category": [["http://snomed.info/sct|16076005"]]},
		}},
	})
}

# Query 16: GET [base]/MedicationDispense?category=http://snomed.info/sct|422037009&_include=MedicationDispense:medication
test_allow_medication_dispense_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "MedicationDispense"},
		"action": {"fhir_rest": {
			"include": ["MedicationDispense:medication"],
			"search_params": {"category": [["http://snomed.info/sct|422037009"]]},
		}},
	})
}

# Query 17: GET [base]/DeviceUseStatement?_include=DeviceUseStatement:device
test_allow_device_use_statement_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "DeviceUseStatement"},
		"action": {"fhir_rest": {"include": ["DeviceUseStatement:device"]}},
	})
}

# Query 18: GET [base]/Immunization?status=completed
test_allow_immunization_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Immunization"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"status": [["completed"]]},
		}},
	})
}

# Query 19: GET [base]/Observation/$lastn?code=http://loinc.org|85354-9
test_allow_observation_lastn_blood_pressure if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"interaction_type": "operation",
			"operation": "lastn",
			"include": [],
			"search_params": {"code": [["http://loinc.org|85354-9"]]},
		}},
	})
}

# Query 20: GET [base]/Observation/$lastn?code=http://loinc.org|8302-2,http://loinc.org|8306-3,http://loinc.org|8308-9
test_allow_observation_lastn_body_measurements if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"interaction_type": "operation",
			"operation": "lastn",
			"include": [],
			"search_params": {"code": [["http://loinc.org|8302-2", "http://loinc.org|8306-3", "http://loinc.org|8308-9"]]},
		}},
	})
}

# Query 21: GET [base]/Observation/$lastn?category=http://snomed.info/sct|275711006&_include=Observation:related-target&_include=Observation:specimen
test_allow_observation_lastn_lab_results if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Observation"},
		"action": {"fhir_rest": {
			"interaction_type": "operation",
			"operation": "lastn",
			"include": ["Observation:related-target", "Observation:specimen"],
			"search_params": {"category": [["http://snomed.info/sct|275711006"]]},
		}},
	})
}

# Query 22: GET [base]/Procedure?category=http://snomed.info/sct|387713003
test_allow_procedure_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Procedure"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"category": [["http://snomed.info/sct|387713003"]]},
		}},
	})
}

# Query 23: GET [base]/Encounter?class=http://hl7.org/fhir/v3/ActCode|IMP,...|ACUTE,...|NONAC
test_allow_encounter_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Encounter"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"class": [[
				"http://hl7.org/fhir/v3/ActCode|IMP",
				"http://hl7.org/fhir/v3/ActCode|ACUTE",
				"http://hl7.org/fhir/v3/ActCode|NONAC",
			]]},
		}},
	})
}

# Query 24: GET [base]/ProcedureRequest?status=active
test_allow_procedure_request_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "ProcedureRequest"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"status": [["active"]]},
		}},
	})
}

# Query 25: GET [base]/ImmunizationRecommendation
test_allow_immunization_recommendation_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "ImmunizationRecommendation"},
		"action": {"fhir_rest": {"include": []}},
	})
}

# Query 26: GET [base]/DeviceRequest?status=active&_include=DeviceRequest:device
test_allow_device_request_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "DeviceRequest"},
		"action": {"fhir_rest": {
			"include": ["DeviceRequest:device"],
			"search_params": {"status": [["active"]]},
		}},
	})
}

# Query 27: GET [base]/Appointment?status=booked,pending,proposed
test_allow_appointment_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Appointment"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"status": [["booked", "pending", "proposed"]]},
		}},
	})
}

# Query 28: GET [base]/DocumentReference?status=current
test_allow_document_reference_search if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "DocumentReference"},
		"action": {"fhir_rest": {
			"include": [],
			"search_params": {"status": [["current"]]},
		}},
	})
}

# Query 29: GET [base]/DocumentReference (read)
test_allow_document_reference_read if {
	bgz.allow with input as object.union(base_input, {
		"resource": {"type": "DocumentReference"},
		"action": {"fhir_rest": {
			"interaction_type": "read",
			"include": [],
		}},
	})
}

# Deny tests

test_deny_wrong_resource_type if {
	not bgz.allow with input as object.union(base_input, {"resource": {"type": "UnknownResource"}})
}

test_deny_condition_with_wrong_interaction_type if {
	not bgz.allow with input as object.union(base_input, {
		"resource": {"type": "Condition"},
		"action": {"fhir_rest": {"interaction_type": "create", "include": []}},
	})
}

test_deny_wrong_search_params if {
	not bgz.allow with input as object.union(base_input, {
		"resource": {"type": "MedicationRequest"},
		"action": {"fhir_rest": {
			"include": ["MedicationRequest:medication"],
			"search_params": {"category": [["http://snomed.info/sct|387713003"]]},
		}},
	})
}
