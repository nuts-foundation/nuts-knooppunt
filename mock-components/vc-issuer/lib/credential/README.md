# Credential Package

Unified credential issuance interface that supports both local signing and Nuts node integration.

## Overview

The credential package provides a single, consistent API for issuing Verifiable Credentials, automatically selecting between local Ed25519 signing or Nuts node delegation based on environment configuration.

## Architecture

```
lib/credential/
├── index.ts      # Main interface - issueCredential()
├── types.ts      # Shared types and configuration
├── local.ts      # Local Ed25519 signing implementation
└── nuts.ts       # Nuts node API integration
```

## Usage

### Basic Usage

```typescript
import { issueCredential } from '@/lib/credential';

const signedCredential = await issueCredential({
  credentialId: 'urn:uuid:12345678-1234-1234-1234-123456789abc',
  issuerDid: 'did:web:issuer.example.com',
  subjectDid: 'did:web:wallet.example.com',
  credentialSubject: {
    organizationName: 'Apotheek De Zonnehoek',
    organizationType: 'pharmacy',
    agbCode: '06010713',
    uraNumber: '32475534',
  },
  context: [
    'https://www.w3.org/2018/credentials/v1',
    'https://issuer.example.com/contexts/vektis-org.jsonld',
  ],
  type: ['VerifiableCredential', 'VektisOrgCredential'],
  issuanceDate: new Date(),
  expirationDate: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000),
});
```

### Mode Detection

The package automatically detects the issuance mode:

```typescript
import { getIssuanceConfig } from '@/lib/credential';

const config = getIssuanceConfig();
console.log(config.mode); // 'local' or 'nuts'
```

## API Reference

### `issueCredential(request: CredentialRequest): Promise<string>`

Main function that issues a credential using the configured method.

**Parameters:**
- `request: CredentialRequest` - Credential request parameters

**Returns:**
- `Promise<string>` - Signed credential as JWT string

**Throws:**
- Error if Nuts node is configured but `NUTS_ISSUER_DID` is not set
- Error if Nuts node API call fails

### `CredentialRequest` Interface

```typescript
interface CredentialRequest {
  credentialId: string;          // Unique credential identifier (e.g., urn:uuid:...)
  issuerDid: string;             // DID of the issuer
  subjectDid: string;            // DID of the credential subject
  credentialSubject: Record<string, unknown>;  // Credential claims
  context: string[];             // JSON-LD context URIs
  type: string[];                // Credential types (e.g., ['VerifiableCredential', 'VektisOrgCredential'])
  issuanceDate: Date;            // When the credential was issued
  expirationDate: Date;          // When the credential expires
}
```

### `getIssuanceConfig(): IssuanceConfig`

Returns the current issuance configuration.

**Returns:**
```typescript
{
  mode: 'local' | 'nuts';
  nutsNodeUrl?: string;
}
```

## Implementation Details

### Local Mode (`local.ts`)

When `NUTS_NODE_INTERNAL_URL` is not set:

1. Retrieves/generates Ed25519 key pair for the issuer DID
2. Builds VC payload with all required fields
3. Signs using jose library (EdDSA algorithm)
4. Returns signed JWT credential

### Nuts Node Mode (`nuts.ts`)

When `NUTS_NODE_INTERNAL_URL` is set:

1. Validates that `NUTS_ISSUER_DID` is configured
2. Calls Nuts node internal API: `POST /internal/vcr/v2/issuer/vc`
3. Receives signed credential from Nuts node
4. Returns the credential

**API Request Format:**
```json
{
  "type": "VektisOrgCredential",
  "issuer": "did:web:example.com:iam:org-a",
  "credentialSubject": {
    "id": "did:web:wallet.example.com",
    "organizationName": "...",
    ...
  },
  "@context": [...],
  "expirationDate": "2026-01-14T12:00:00Z"
}
```

**API Response Format:**
```json
{
  "credential": "eyJhbGciOiJFZERTQSIs..."
}
```

## Configuration

### Local Signing (Default)

No special configuration needed. Uses `ISSUER_HOSTNAME` to generate did:web:

```bash
ISSUER_HOSTNAME=localhost:3000
```

### Nuts Node Integration

Requires both environment variables:

```bash
NUTS_NODE_INTERNAL_URL=http://nuts-node:8081
NUTS_ISSUER_DID=did:web:example.com:iam:org-a
```

## Error Handling

### Configuration Errors

```typescript
// Missing NUTS_ISSUER_DID when Nuts node is configured
throw new Error('NUTS_ISSUER_DID is required when using Nuts node for credential issuance');
```

### Nuts Node API Errors

```typescript
// HTTP error from Nuts node
throw new Error('Failed to issue credential via Nuts node: 500 Internal Server Error');
```

### Local Signing Errors

Errors from jose library (e.g., invalid key format, signing failures)

## Logging

Both implementations provide detailed logging:

**Local mode:**
```
[CredentialIssuer] Mode: local
[LocalIssuer] Signing credential locally
[LocalIssuer] Credential ID: urn:uuid:...
[LocalIssuer] Issuer DID: did:web:...
[LocalIssuer] Credential signed successfully
```

**Nuts mode:**
```
[CredentialIssuer] Mode: nuts
[NutsIssuer] Issuing credential via Nuts node
[NutsIssuer] Endpoint: http://nuts-node:8081/internal/vcr/v2/issuer/vc
[NutsIssuer] Issuer DID: did:web:example.com:iam:org-a
[NutsIssuer] Request: { ... }
[NutsIssuer] Successfully issued credential via Nuts node
```

## Migration Guide

### From Old API

**Before:**
```typescript
import { signCredential } from '@/lib/crypto/signing';
import { issueCredentialViaNuts } from '@/lib/nuts/client';

// Manual mode selection
if (useNutsNode) {
  credential = await issueCredentialViaNuts(nutsNodeUrl, {...});
} else {
  credential = await signCredential(payload, issuerDid, subjectDid, days);
}
```

**After:**
```typescript
import { issueCredential } from '@/lib/credential';

// Automatic mode selection
credential = await issueCredential({
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

## Benefits

✅ **Unified Interface**: Single function with consistent parameters  
✅ **Automatic Selection**: No manual mode detection needed  
✅ **Type Safety**: Shared `CredentialRequest` type for both modes  
✅ **Clean Separation**: Local and Nuts implementations are isolated  
✅ **Easy Testing**: Mock either implementation independently  
✅ **Future-Proof**: Easy to add more issuance methods  

## Future Enhancements

- Support for additional credential formats (e.g., JSON-LD)
- Batch credential issuance
- Caching layer for Nuts node responses
- Retry logic with exponential backoff
- Health checks for Nuts node connectivity

