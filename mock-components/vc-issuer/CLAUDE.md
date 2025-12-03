# VC Issuer - Claude Code Instructions

## Project Overview

This is an OID4VCI-compliant Verifiable Credential issuer for VektisOrgCredentials. It implements the wallet-initiated Authorization Code Flow with mock e-Herkenning authentication.

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
  - `crypto/` - Ed25519 keys, DID:web, JWT signing
  - `oid4vci/` - PKCE, metadata definitions
  - `mock-data/` - Test organizations
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

## Testing

Tests are in `lib/**/*.test.ts`. Run with `npm test`.

## Environment Variables

```
DATABASE_URL=file:./dev.db
NEXT_PUBLIC_BASE_URL=http://localhost:3000  # Fallback only
CREDENTIAL_VALIDITY_DAYS=365
ACCESS_TOKEN_EXPIRY_SECONDS=86400
C_NONCE_EXPIRY_SECONDS=86400
```
