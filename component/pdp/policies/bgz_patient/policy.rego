package bgz_patient

import rego.v1
import data.bgz

default allow := false

allow if {
    input.context.mitz_consent
    bgz.is_allowed_query
}