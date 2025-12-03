# Vektis VC Issuer

OID4VCI-compliant Verifiable Credential Issuer for VektisOrgCredentials.

## Overview

This service implements the [OpenID for Verifiable Credential Issuance (OID4VCI)](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0.html) specification to issue VektisOrgCredentials to wallets. It uses the wallet-initiated Authorization Code Flow with PKCE.

**Issue Reference:** [#196 - Implement Vektis-Organisatie-Type-Credential](https://github.com/nuts-foundation/nuts-knooppunt/issues/196)

## Features

- OID4VCI Authorization Code Flow (wallet-initiated)
- PKCE (S256) for secure code exchange
- Mock e-Herkenning authentication
- Ed25519 (EdDSA) credential signing
- JWT VC format (`jwt_vc_json`)
- DID:web for issuer identity
- PostgreSQL for data persistence

## Quick Start

### Prerequisites

- Node.js 20+
- Docker and Docker Compose
- PostgreSQL (or use Docker)

### Development Setup

1. **Start PostgreSQL:**

```bash
docker-compose up -d postgres
```

2. **Install dependencies:**

```bash
npm install
```

3. **Run database migrations:**

```bash
npm run db:push
```

4. **Start development server:**

```bash
npm run dev
```

The service will be available at http://localhost:3000.

### Using Docker Compose

```bash
docker-compose up --build
```

## API Endpoints

### Discovery Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /.well-known/openid-credential-issuer` | Credential issuer metadata |
| `GET /.well-known/openid-configuration` | OAuth2 authorization server metadata |
| `GET /.well-known/did.json` | DID document for issuer identity |

### OID4VCI Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/oidc4vci/authorize` | GET | Authorization endpoint |
| `/api/oidc4vci/token` | POST | Token endpoint |
| `/api/oidc4vci/credential` | POST | Credential issuance endpoint |

## OID4VCI Flow

```
Wallet                         Issuer                      e-Herkenning (Mock)
  │                              │                              │
  │──1. GET /.well-known/openid-credential-issuer──────────────►│
  │◄──── Issuer Metadata ────────│                              │
  │                              │                              │
  │──2. GET /api/oidc4vci/authorize ─────────────────────────►  │
  │    (response_type, client_id, redirect_uri,                 │
  │     code_challenge, authorization_details)                  │
  │                              │──3. Redirect ───────────────►│
  │                              │                              │
  │                              │◄─4. User selects org ────────│
  │◄──5. Redirect with code ─────│                              │
  │                              │                              │
  │──6. POST /api/oidc4vci/token ─────────────────────────────► │
  │    (code, code_verifier)     │                              │
  │◄──── access_token, c_nonce ──│                              │
  │                              │                              │
  │──7. POST /api/oidc4vci/credential ────────────────────────► │
  │    (proof JWT with c_nonce)  │                              │
  │◄──── JWT VC ─────────────────│                              │
```

## VektisOrgCredential

The issued credential contains:

```json
{
  "vc": {
    "@context": ["https://www.w3.org/2018/credentials/v1"],
    "type": ["VerifiableCredential", "VektisOrgCredential"],
    "credentialSubject": {
      "id": "did:web:wallet.example.com",
      "organizationName": "Apotheek De Zonnehoek",
      "organizationType": "pharmacy",
      "agbCode": "06010713",
      "uraNumber": "32475534"
    },
    "issuer": "did:web:issuer.example.com",
    "issuanceDate": "2024-12-01T12:00:00Z"
  }
}
```

## Configuration

Environment variables (see `.env.example`):

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | - |
| `NEXT_PUBLIC_BASE_URL` | Public URL of the service | `http://localhost:3000` |
| `ISSUER_HOSTNAME` | Hostname for DID:web | `localhost:3000` |
| `CREDENTIAL_VALIDITY_DAYS` | Credential validity period | `365` |
| `ACCESS_TOKEN_EXPIRY_SECONDS` | Access token TTL | `86400` |
| `C_NONCE_EXPIRY_SECONDS` | c_nonce TTL | `86400` |

## Mock Organizations

The mock e-Herkenning provides these test organizations:

| Name | Type | AGB Code | URA Number |
|------|------|----------|------------|
| Apotheek De Zonnehoek | Pharmacy | 06010713 | 32475534 |
| Huisartsenpraktijk Centrum | General Practice | 01234567 | 12345678 |
| Ziekenhuis Oost | Hospital | 98765432 | 87654321 |
| Verpleeghuis De Rusthoeve | Care Home | 11223344 | 44332211 |

## Development

### Database Management

```bash
# Push schema to database
npm run db:push

# Run migrations
npm run db:migrate

# Open Prisma Studio
npm run db:studio
```

### Code Formatting

```bash
npm run format
```

## Technology Stack

- **Framework:** Next.js 16 with TypeScript (App Router)
- **Database:** PostgreSQL with Prisma ORM
- **Crypto:** jose library (Ed25519/EdDSA)
- **Styling:** Tailwind CSS

## License

MIT
