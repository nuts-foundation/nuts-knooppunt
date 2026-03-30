package eoverdracht_receiver_test

import rego.v1

import data.eoverdracht_receiver

base_input := {
	"subject": {"organization": {"ura": "12345"}},
	"action": {"request": {"method": "POST", "path": "/"}},
}

test_allow_post_root_slash if {
	eoverdracht_receiver.allow with input as base_input
}

test_allow_post_root_empty if {
	eoverdracht_receiver.allow with input as object.union(base_input, {"action": {"request": {"path": ""}}})
}

test_deny_empty_ura if {
	not eoverdracht_receiver.allow with input as object.union(base_input, {"subject": {"organization": {"ura": ""}}})
}

test_deny_wrong_method if {
	not eoverdracht_receiver.allow with input as object.union(base_input, {"action": {"request": {"method": "GET"}}})
}

test_deny_wrong_path if {
	not eoverdracht_receiver.allow with input as object.union(base_input, {"action": {"request": {"path": "/other"}}})
}

test_deny_missing_subject if {
	not eoverdracht_receiver.allow with input as {"action": {"request": {"method": "POST", "path": "/"}}}
}
