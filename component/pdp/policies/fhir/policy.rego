package fhir

import rego.v1

# Supported FHIR RESTful interaction types
supported_interactions := [
    "read",
    "vread",
    "update",
    "patch",
    "delete",
    "history-instance",
    "history-type",
    "create",
    "search-type"
]

# capability_statement_allowed evaluates whether a FHIR request conforms to the provided capability statement.
# Returns true if the request is allowed, false otherwise.
capability_statement_allowed(capability_statement, resource_type, fhir_rest) if {
    capability_statement
    interaction_supported(fhir_rest.interaction_type)
    interaction_allowed(capability_statement, resource_type, fhir_rest.interaction_type)
    search_params_allowed(capability_statement, resource_type, fhir_rest)
    includes_allowed(capability_statement, resource_type, fhir_rest.include)
    revincludes_allowed(capability_statement, resource_type, fhir_rest.revinclude)
}

# Check if interaction type is supported
interaction_supported(interaction_type) if {
    interaction_type in supported_interactions
}

# Check if the interaction is allowed by the capability statement
interaction_allowed(capability_statement, resource_type, interaction_type) if {
    some rest in capability_statement.rest
    some resource in rest.resource
    resource.type == resource_type
    some interaction in resource.interaction
    interaction.code == interaction_type
}

# Check if search parameters are allowed (only applies to search-type interactions)
default search_params_allowed(capability_statement, resource_type, fhir_rest) := true

search_params_allowed(capability_statement, resource_type, fhir_rest) if {
    fhir_rest.interaction_type == "search-type"
    allowed_params := {param.name |
        some rest in capability_statement.rest
        some resource in rest.resource
        resource.type == resource_type
        some param in resource.searchParam
    }
    # All search params must be in the allowed list
    rejected_params := {param_name |
        some param_name in object.keys(fhir_rest.search_params)
        not param_name in allowed_params
    }
    count(rejected_params) == 0
}

# Check if includes are allowed
default includes_allowed(capability_statement, resource_type, includes) := true

includes_allowed(capability_statement, resource_type, includes) if {
    count(includes) > 0
    allowed_includes := {include |
        some rest in capability_statement.rest
        some resource in rest.resource
        resource.type == resource_type
        some include in resource.searchInclude
    }
    # All includes must be in the allowed list
    rejected_includes := {inc |
        some inc in includes
        not inc in allowed_includes
    }
    count(rejected_includes) == 0
}

# Check if revincludes are allowed
default revincludes_allowed(capability_statement, resource_type, revincludes) := true

revincludes_allowed(capability_statement, resource_type, revincludes) if {
    count(revincludes) > 0
    allowed_revincludes := {revinclude |
        some rest in capability_statement.rest
        some resource in rest.resource
        resource.type == resource_type
        some revinclude in resource.searchRevInclude
    }
    # All revincludes must be in the allowed list
    rejected_revincludes := {revinc |
        some revinc in revincludes
        not revinc in allowed_revincludes
    }
    count(rejected_revincludes) == 0
}

