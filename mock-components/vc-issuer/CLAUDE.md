# VC Issuer - Claude Code Instructions

## Project Overview

This is an OID4VCI-compliant Verifiable Credential issuer for HealthcareProviderRoleTypeCredentials. It implements the wallet-initiated Authorization Code Flow with mock e-Herkenning authentication.

## Tech Stack

- **Framework:** Next.js 16 with App Router, TypeScript
- **Database:** SQLite via Prisma ORM
- **Crypto:** jose library for Ed25519/EdDSA signing
- **Styling:** Tailwind CSS

## Common Commands

```bash
npm run dev          # Start development server
npm run build        # Build for production
npm test             # Run Jest tests
npm run db:push      # Apply Prisma schema to database
npm run db:studio    # Open Prisma Studio
```

## Project Structure

- `app/` - Next.js App Router pages and API routes
  - `.well-known/` - Discovery endpoints (DID, OAuth metadata)
  - `api/oidc4vci/` - OID4VCI endpoints (authorize, token, credential)
  - `e-herkenning/` - Mock authentication UI
- `lib/` - Shared libraries
  - `credential/` - **Unified credential issuance** (local + Nuts node)
    - `index.ts` - Main interface with `issueCredential()`
    - `local.ts` - Local Ed25519 signing implementation
    - `nuts.ts` - Nuts node API integration
    - `types.ts` - Shared types and configuration
  - `crypto/` - Ed25519 keys, DID:web, JWT signing (legacy)
  - `oid4vci/` - PKCE, metadata definitions
  - `mock-data/` - Test organizations
  - `nuts/` - (Deprecated) Use `lib/credential` instead
- `prisma/` - Database schema

## Key Patterns

### Database
- SQLite stores JSON as strings - use `JSON.stringify()` when writing, `JSON.parse()` when reading
- Prisma client singleton in `lib/prisma.ts`

### URLs
- URLs are derived from request headers (`host`, `x-forwarded-host`, `x-forwarded-proto`)
- This supports reverse proxy deployments
- Use `getBaseUrl(req)` and `getIssuerDid(req)` from `lib/utils.ts`

### Credentials
- Format: `jwt_vc_json` (JWT Verifiable Credential)
- Signing: Ed25519/EdDSA
- Identity: DID:web based on hostname

### Credential Issuance
- **Unified Interface**: Use `issueCredential()` from `@/lib/credential`
- **Automatic Mode Selection**: Detects local vs Nuts node based on environment
- **Single Signature**: Both local and Nuts implementations accept `CredentialRequest` interface
- **Legacy Support**: Old `signCredential()` function is deprecated but still available

Example usage:
```typescript
import { issueCredential } from '@/lib/credential';

const credential = await issueCredential({
  credentialId: 'urn:uuid:...',
  issuerDid: 'did:web:issuer.example.com',
  subjectDid: 'did:web:wallet.example.com',
  credentialSubject: { organizationName: '...' },
  context: ['https://www.w3.org/2018/credentials/v1'],
  type: ['VerifiableCredential', 'VektisOrgCredential'],
  issuanceDate: new Date(),
  expirationDate: new Date(),
});
```

## Testing

Tests are in `lib/**/*.test.ts`. Run with `npm test`.

## Environment Variables

```
DATABASE_URL=file:./dev.db
ISSUER_HOSTNAME=localhost:3000  # Used for DID:web identity and base URL derivation
CREDENTIAL_VALIDITY_DAYS=365
ACCESS_TOKEN_EXPIRY_SECONDS=86400
C_NONCE_EXPIRY_SECONDS=86400

# Optional Nuts Node Integration
NUTS_NODE_INTERNAL_URL=http://nuts-node:8081  # If set, credentials are issued via Nuts node
NUTS_ISSUER_DID=did:web:example.com:iam:org-a  # Required when using Nuts node integration
```

Note: `ISSUER_HOSTNAME` is the single source of truth for the issuer's identity. The base URL is derived from it (https for non-localhost, http for localhost). In production, request headers (`host`, `x-forwarded-host`, `x-forwarded-proto`) take precedence.

When `NUTS_NODE_INTERNAL_URL` is configured, the issuer delegates credential signing to the Nuts node instead of signing locally. In this mode, `NUTS_ISSUER_DID` is required and will be used as the issuer DID instead of the did:web derived from the hostname.

## Nuts Node Integration

The issuer supports two modes of operation:

1. **Local Signing (default)**: Credentials are signed using locally generated Ed25519 keys, with a did:web identifier derived from the hostname.

2. **Nuts Node Issuance**: When `NUTS_NODE_INTERNAL_URL` is configured, credentials are issued via the Nuts node's internal VCR API (`/internal/vcr/v2/issuer/vc`). This requires:
   - `NUTS_NODE_INTERNAL_URL`: Internal API endpoint (e.g., `http://nuts-node:8081`)
   - `NUTS_ISSUER_DID`: The DID to use as issuer (e.g., `did:web:example.com:iam:org-a`)

The mode is automatically selected based on configuration - no code changes needed.

