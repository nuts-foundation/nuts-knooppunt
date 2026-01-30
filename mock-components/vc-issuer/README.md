# Vektis VC Issuer

OID4VCI-compliant Verifiable Credential Issuer for HealthcareProviderRoleTypeCredentials.

## Overview

This service implements the [OpenID for Verifiable Credential Issuance (OID4VCI)](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0.html) specification to issue HealthcareProviderRoleTypeCredentials to wallets. It uses the wallet-initiated Authorization Code Flow with PKCE.

**Issue Reference:** [#196 - Implement Vektis-Organisatie-Type-Credential](https://github.com/nuts-foundation/nuts-knooppunt/issues/196)

## Features

- OID4VCI Authorization Code Flow (wallet-initiated)
- PKCE (S256) for secure code exchange
- Mock e-Herkenning authentication
- Ed25519 (EdDSA) credential signing
- JWT VC format (`jwt_vc_json`)
- DID:web for issuer identity
- Optional Nuts node integration for credential issuance

## Quick Start

### Prerequisites

- Node.js 20+
- Docker and Docker Compose

### Development Setup

1. **Install dependencies:**

```bash
npm install
```

2. **Run database migrations:**

```bash
npm run db:push
```

3. **Start development server:**

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

| Endpoint                                    | Description                          |
|---------------------------------------------|--------------------------------------|
| `GET /.well-known/openid-credential-issuer` | Credential issuer metadata           |
| `GET /.well-known/openid-configuration`     | OAuth2 authorization server metadata |
| `GET /.well-known/did.json`                 | DID document for issuer identity     |

### OID4VCI Endpoints

| Endpoint                   | Method | Description                  |
|----------------------------|--------|------------------------------|
| `/api/oidc4vci/authorize`  | GET    | Authorization endpoint       |
| `/api/oidc4vci/token`      | POST   | Token endpoint               |
| `/api/oidc4vci/credential` | POST   | Credential issuance endpoint |

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

## HealthcareProviderRoleTypeCredential

The issued credential contains:

```json
{
  "vc": {
    "@context": ["https://www.w3.org/2018/credentials/v1"],
    "type": ["VerifiableCredential", "HealthcareProviderRoleTypeCredential"],
    "credentialSubject": {
      "id": "did:web:wallet.example.com",
      "roleCodeNL": "A1"
    },
    "issuer": "did:web:issuer.example.com",
    "issuanceDate": "2024-12-01T12:00:00Z"
  }
}
```

## Configuration

Environment variables (see `.env.example`):

| Variable                      | Description                                | Default                 |
|-------------------------------|--------------------------------------------|-------------------------|
| `DATABASE_URL`                | Connection string                          | -                       |
| `NEXT_PUBLIC_BASE_URL`        | Public URL of the service                  | `http://localhost:3000` |
| `ISSUER_HOSTNAME`             | Hostname for DID:web                       | `localhost:3000`        |
| `CREDENTIAL_VALIDITY_DAYS`    | Credential validity period                 | `365`                   |
| `ACCESS_TOKEN_EXPIRY_SECONDS` | Access token TTL                           | `86400`                 |
| `C_NONCE_EXPIRY_SECONDS`      | c_nonce TTL                                | `86400`                 |
| `NUTS_NODE_INTERNAL_URL`      | (Optional) Nuts node internal API URL      | -                       |
| `NUTS_ISSUER_DID`             | (Optional) Issuer DID when using Nuts node | -                       |

### Nuts Node Integration

By default, the issuer signs credentials locally using Ed25519 keys. However, you can optionally configure it to use a
Nuts node for credential issuance:

1. Set `NUTS_NODE_INTERNAL_URL` to the internal API endpoint of your Nuts node (e.g., `http://nuts-node:8081`)
2. Set `NUTS_ISSUER_DID` to the DID that should be used as the issuer (e.g., `did:web:example.com:iam:org-a`)

When both variables are configured, the issuer will delegate credential signing to the Nuts node instead of signing
locally.

**Example docker-compose configuration:**

```yaml
environment:
  NUTS_NODE_INTERNAL_URL: http://nuts-node:8081
  NUTS_ISSUER_DID: did:web:example.com:iam:org-a
```

**Note:** `NUTS_ISSUER_DID` is required when using Nuts node integration. If `NUTS_NODE_INTERNAL_URL` is set but
`NUTS_ISSUER_DID` is not, credential issuance will fail.

## Mock e-Herkenning

The mock e-Herkenning allows you to manually enter organization details during the authentication flow.

### Issuing a Credential

To issue a credential during the e-Herkenning flow:

1. Select the healthcare provider type from the dropdown with 96 official Vektis categories
2. Click "Doorgaan" (Continue) to proceed with the credential issuance

The healthcare provider types are based on the official Vektis "Dossierhoudende zorgaanbiedercategorieën" (Dossier-holding healthcare provider categories).

**Note:** The credential only contains the `roleCodeNL` code (e.g., "A1", "H1"). No organization name or other identifying information is included.

Source: [Vektis - Dossierhoudende zorgaanbiedercategorieën](https://vzvz.atlassian.net/wiki/spaces/MA11/pages/828314634/Bijlage+Dossierhoudende+zorgaanbiedercategorie+n)

This feature allows testing with custom organization data without modifying the code.

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
- **Database:** SQLite
- **Crypto:** jose library (Ed25519/EdDSA)
- **Styling:** Tailwind CSS

## License

MIT
