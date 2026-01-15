# Bundles Directory Relocation

## Summary

Successfully moved the OPA bundles directory from `component/pdp/opa/bundles/` to `component/pdp/opa/policies/bundles/` for better organization.

## Changes Made

### 1. Directory Structure
- **Before:** `component/pdp/opa/bundles/*.tar.gz`
- **After:** `component/pdp/opa/policies/bundles/*.tar.gz`

### 2. Files Modified

#### `component/pdp/opa/bundles.go`
- Updated `go:embed` directive: `bundles/*.tar.gz` → `policies/bundles/*.tar.gz`
- Updated `ReadDir()` call: `"bundles"` → `"policies/bundles"`
- Updated `filepath.Join()` calls to use new path

#### `component/pdp/opa/cmd/generate_bundles/main.go`
- Updated `bundlesDir` variable: `"bundles"` → `"policies/bundles"`

#### Documentation
- Updated `component/pdp/opa/README.md` directory structure diagram
- Updated `OPA_INTEGRATION_COMPLETE.md` file tree
- Added note about the relocation in Known Issues Fixed section

### 3. Verification

✅ All tests pass:
```
ok  github.com/nuts-foundation/nuts-knooppunt/component/pdp     0.501s  coverage: 55.9%
ok  github.com/nuts-foundation/nuts-knooppunt/component/pdp/opa 0.866s  coverage: 83.3%
```

✅ Build succeeds:
```
go build ./component/pdp/...
```

✅ go:generate works:
```
cd component/pdp/opa && go generate
```

## Benefits

1. **Better Organization:** Bundles are now co-located with their source policies
2. **Clearer Structure:** The `policies/` directory is now self-contained
3. **Easier Navigation:** Related files (policies and bundles) are in the same directory tree

## Directory Structure (After)

```
component/pdp/opa/
├── policies/
│   ├── bundles/               # Generated bundles
│   │   ├── bgz_patient.tar.gz
│   │   ├── bgz_professional.tar.gz
│   │   ├── mcsd_query.tar.gz
│   │   └── mcsd_update.tar.gz
│   ├── bgz_patient.rego       # Policy source
│   ├── bgz_professional.rego
│   ├── mcsd_query.rego
│   └── mcsd_update.rego
├── cmd/
│   └── generate_bundles/
│       └── main.go
├── bundles.go
├── service.go
├── service_test.go
└── README.md
```

## Status: ✅ COMPLETE

