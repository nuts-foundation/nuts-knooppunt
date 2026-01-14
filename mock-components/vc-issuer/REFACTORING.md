# Credential Package Refactoring Summary

## Overview

Refactored credential issuance logic from scattered implementations into a unified `lib/credential` package with a consistent interface for both local signing and Nuts node integration.

## Changes Made

### New Files Created

#### 1. `lib/credential/types.ts`
- Defines `CredentialRequest` interface with all credential parameters
- Defines `IssuanceConfig` interface for configuration
- Exports `getIssuanceConfig()` function for automatic mode detection

#### 2. `lib/credential/local.ts`
- `issueCredentialLocally(request: CredentialRequest): Promise<string>`
- Implements local Ed25519 signing using jose library
- Accepts unified `CredentialRequest` parameter

#### 3. `lib/credential/nuts.ts`
- `issueCredentialViaNuts(request: CredentialRequest, nutsNodeUrl: string): Promise<string>`
- Implements Nuts node API integration
- Accepts unified `CredentialRequest` parameter
- Calls `/internal/vcr/v2/issuer/vc` endpoint

#### 4. `lib/credential/index.ts`
- Main entry point with `issueCredential(request: CredentialRequest): Promise<string>`
- Automatically selects local or Nuts mode based on configuration
- Validates configuration (e.g., requires NUTS_ISSUER_DID for Nuts mode)
- Re-exports types and utilities

#### 5. `lib/credential/README.md`
- Comprehensive documentation for the credential package
- API reference, usage examples, migration guide
- Architecture overview and implementation details

### Modified Files

#### 1. `app/api/oidc4vci/credential/route.ts`
**Before:**
```typescript
import { signCredential, ... } from '@/lib/crypto/signing';
import { issueCredentialViaNuts, ... } from '@/lib/nuts/client';

// Manual mode selection with lots of boilerplate
if (useNutsNode) {
  const nutsNodeUrl = getNutsNodeUrl();
  const nutsIssuerDid = getNutsIssuerDid();
  // validation...
  signedCredential = await issueCredentialViaNuts(nutsNodeUrl, {
    type: 'VektisOrgCredential',
    issuer: nutsIssuerDid,
    // ... manual payload construction
  });
} else {
  signedCredential = await signCredential(
    credentialPayload,
    issuerDid,
    subjectDid,
    validityDays
  );
}
```

**After:**
```typescript
import { issueCredential } from '@/lib/credential';

// Automatic mode selection, clean interface
signedCredential = await issueCredential({
  credentialId,
  issuerDid,
  subjectDid,
  credentialSubject,
  context,
  type,
  issuanceDate,
  expirationDate,
});
```

**Benefits:**
- Removed ~50 lines of mode detection and validation code
- Single function call with clear parameters
- No conditional logic needed in the route handler
- Better type safety with `CredentialRequest` interface

#### 2. `lib/crypto/signing.ts`
- Added `@deprecated` JSDoc tag to `signCredential()` function
- Function remains for backward compatibility
- Directs developers to use `issueCredential` from `@/lib/credential`

#### 3. `CLAUDE.md`
- Updated project structure to highlight new `lib/credential/` package
- Added "Credential Issuance" section with usage examples
- Marked `lib/nuts/` as deprecated

## Architecture

### Before Refactoring

```
Scattered implementation:
- lib/crypto/signing.ts      (local signing)
- lib/nuts/client.ts          (Nuts node integration)
- Manual mode selection in route handlers
```

### After Refactoring

```
Unified package:
lib/credential/
├── index.ts       # Public API - issueCredential()
├── types.ts       # Shared interfaces
├── local.ts       # Local implementation
├── nuts.ts        # Nuts implementation
└── README.md      # Documentation
```

## Key Improvements

### 1. **Unified Signature**
Both implementations now accept the same `CredentialRequest` interface:

```typescript
interface CredentialRequest {
  credentialId: string;
  issuerDid: string;
  subjectDid: string;
  credentialSubject: Record<string, unknown>;
  context: string[];
  type: string[];
  issuanceDate: Date;
  expirationDate: Date;
}
```

### 2. **Automatic Mode Selection**
Configuration-based mode detection in one place:

```typescript
export function getIssuanceConfig(): IssuanceConfig {
  const nutsNodeUrl = process.env.NUTS_NODE_INTERNAL_URL;
  return nutsNodeUrl 
    ? { mode: 'nuts', nutsNodeUrl }
    : { mode: 'local' };
}
```

### 3. **Centralized Validation**
All validation logic is now in `lib/credential/index.ts`:

```typescript
if (config.mode === 'nuts') {
  if (!config.nutsNodeUrl) {
    throw new Error('NUTS_NODE_INTERNAL_URL is not configured');
  }
  if (!process.env.NUTS_ISSUER_DID) {
    throw new Error('NUTS_ISSUER_DID is required...');
  }
}
```

### 4. **Better Separation of Concerns**
- `local.ts` - Only local signing logic
- `nuts.ts` - Only Nuts API integration
- `index.ts` - Orchestration and validation
- `types.ts` - Shared contracts

### 5. **Improved Testability**
Each implementation can be tested independently:

```typescript
// Test local signing
import { issueCredentialLocally } from '@/lib/credential/local';

// Test Nuts integration
import { issueCredentialViaNuts } from '@/lib/credential/nuts';

// Test orchestration
import { issueCredential } from '@/lib/credential';
```

## Migration Path

### For New Code
Use the new package directly:
```typescript
import { issueCredential } from '@/lib/credential';
```

### For Existing Code
The old API is still available but deprecated:
```typescript
import { signCredential } from '@/lib/crypto/signing'; // Still works
```

### Old `lib/nuts/client.ts`
Functions are no longer needed:
- `issueCredentialViaNuts()` → moved to `lib/credential/nuts.ts`
- `isNutsNodeEnabled()` → use `getIssuanceConfig()`
- `getNutsNodeUrl()` → handled internally
- `getNutsIssuerDid()` → handled internally

The file can be deprecated or removed in a future release.

## Code Reduction

### Route Handler
- **Before**: ~70 lines for credential issuance logic
- **After**: ~20 lines
- **Reduction**: ~50 lines (~70% fewer)

### Duplicated Logic
- **Before**: Mode detection and validation in route handler
- **After**: Centralized in credential package
- **Benefit**: Single source of truth

## Testing Strategy

### Unit Tests
```typescript
describe('issueCredentialLocally', () => {
  it('should sign credential with Ed25519', async () => {
    // Test local signing in isolation
  });
});

describe('issueCredentialViaNuts', () => {
  it('should call Nuts node API', async () => {
    // Test Nuts integration with mocked fetch
  });
});

describe('issueCredential', () => {
  it('should use local mode when NUTS_NODE_INTERNAL_URL is not set', async () => {
    // Test orchestration
  });
  
  it('should use Nuts mode when NUTS_NODE_INTERNAL_URL is set', async () => {
    // Test orchestration
  });
});
```

### Integration Tests
```typescript
describe('Credential Route', () => {
  it('should issue credential in local mode', async () => {
    // End-to-end test with local signing
  });
  
  it('should issue credential via Nuts node', async () => {
    // End-to-end test with Nuts mock
  });
});
```

## Benefits Summary

✅ **Unified Interface**: Single function signature for both modes  
✅ **Better Maintainability**: Logic organized in dedicated package  
✅ **Reduced Complexity**: Route handlers are simpler and cleaner  
✅ **Type Safety**: Shared `CredentialRequest` type enforces consistency  
✅ **Testability**: Each component can be tested independently  
✅ **Extensibility**: Easy to add new issuance methods  
✅ **Documentation**: Comprehensive docs in `lib/credential/README.md`  
✅ **Backward Compatible**: Old API still works (deprecated)  

## Files Summary

**Created:**
- `lib/credential/index.ts` (main interface)
- `lib/credential/types.ts` (shared types)
- `lib/credential/local.ts` (local signing)
- `lib/credential/nuts.ts` (Nuts integration)
- `lib/credential/README.md` (documentation)

**Modified:**
- `app/api/oidc4vci/credential/route.ts` (simplified route handler)
- `lib/crypto/signing.ts` (deprecated old function)
- `CLAUDE.md` (updated developer docs)

**Deprecated/Can be removed:**
- `lib/nuts/client.ts` (functionality moved to credential package)

## Next Steps

1. ✅ Refactoring complete
2. ⏭️ Add unit tests for credential package
3. ⏭️ Update other route handlers if they use old API
4. ⏭️ Consider removing `lib/nuts/client.ts` in future release
5. ⏭️ Add JSDoc comments to all public functions

