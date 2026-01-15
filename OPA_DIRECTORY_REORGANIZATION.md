# OPA Directory Reorganization Summary

## Changes Completed

Successfully reorganized the OPA directory structure to improve organization and clarity.

### 1. Policy Organization (Per-Policy Directories)

**Before:**
```
policies/
├── mcsd_update.rego
├── mcsd_query.rego
├── bgz_patient.rego
├── bgz_professional.rego
└── bundles/
```

**After:**
```
policies/
├── mcsd_update/
│   ├── policy.rego
│   └── capability.json
├── mcsd_query/
│   ├── policy.rego
│   └── capability.json
├── bgz_patient/
│   ├── policy.rego
│   └── capability.json
├── bgz_professional/
│   ├── policy.rego
│   └── capability.json
└── bundles/
```

**Benefits:**
- Each policy is self-contained in its own directory
- All policy files are consistently named `policy.rego`
- Capability statements are co-located with their policies
- Easier to add new policies (just create a new directory)

### 2. Bundle Generator Location

**Before:** `component/pdp/opa/cmd/generate_bundles/`
**After:** `component/pdp/opa/bundles/generate_bundles/`

**Benefits:**
- Generator is now logically grouped with the bundles it generates
- Clearer that this tool is specifically for bundle generation

### 3. Files Modified

#### Code Changes:
- ✅ `bundles.go` - Updated go:generate directive: `cmd/generate_bundles` → `bundles/generate_bundles`
- ✅ `bundles/generate_bundles/main.go` - Updated to scan policy subdirectories for `policy.rego` files
- ✅ Moved all `.rego` files to their respective directories and renamed to `policy.rego`
- ✅ Copied capability statements from `capabilities/` to policy directories

#### Documentation:
- ✅ `README.md` - Updated directory structure and examples
- ✅ `OPA_INTEGRATION_COMPLETE.md` - Updated file tree

### 4. Bundle Contents

Each bundle now includes:
- `.manifest` - OPA bundle manifest
- `policy.rego` - The policy rules
- `capability.json` - FHIR capability statement (if present)

Example:
```bash
$ tar -tzf policies/bundles/mcsd_update.tar.gz
.manifest
policy.rego
capability.json
```

### 5. Final Directory Structure

```
component/pdp/opa/
├── policies/
│   ├── mcsd_update/
│   │   ├── policy.rego
│   │   └── capability.json
│   ├── mcsd_query/
│   │   ├── policy.rego
│   │   └── capability.json
│   ├── bgz_patient/
│   │   ├── policy.rego
│   │   └── capability.json
│   ├── bgz_professional/
│   │   ├── policy.rego
│   │   └── capability.json
│   └── bundles/
│       ├── mcsd_update.tar.gz
│       ├── mcsd_query.tar.gz
│       ├── bgz_patient.tar.gz
│       └── bgz_professional.tar.gz
├── bundles/
│   └── generate_bundles/
│       └── main.go
├── bundles.go
├── service.go
├── service_test.go
└── README.md
```

### 6. Verification

✅ All tests pass:
```
ok  github.com/nuts-foundation/nuts-knooppunt/component/pdp     0.472s
ok  github.com/nuts-foundation/nuts-knooppunt/component/pdp/opa (cached)
```

✅ Build succeeds:
```
go build ./component/pdp/...
```

✅ go:generate works:
```
cd component/pdp/opa && go generate
Generating OPA policy bundles...
  Building bundle: bgz_patient
  Building bundle: bgz_professional
  Building bundle: mcsd_query
  Building bundle: mcsd_update
Bundle generation complete!
```

### 7. Usage

To add a new policy:
```bash
# Create policy directory
mkdir component/pdp/opa/policies/new_scope

# Create policy file
cat > component/pdp/opa/policies/new_scope/policy.rego << 'EOF'
package knooppunt.new_scope

import rego.v1

default allow := false

allow if {
    # Your rules here
}
EOF

# (Optional) Add capability statement
cp capability.json component/pdp/opa/policies/new_scope/

# Regenerate bundles
cd component/pdp/opa && go generate
```

## Status: ✅ COMPLETE

All changes have been successfully implemented, tested, and documented. The OPA directory structure is now more organized and easier to maintain.

