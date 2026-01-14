# Nuts Node Integration

This document describes the Nuts node integration feature for the VC Issuer.

## Overview

The VC Issuer now supports two modes of credential issuance:

1. **Local Signing (default)**: Credentials are signed using locally generated Ed25519 keys with a did:web identifier
2. **Nuts Node Issuance**: Credentials are issued via a Nuts node's internal VCR API

## Configuration

### Environment Variables

| Variable                 | Required                 | Description                       | Example                         |
|--------------------------|--------------------------|-----------------------------------|---------------------------------|
| `NUTS_NODE_INTERNAL_URL` | No                       | Internal API URL of the Nuts node | `http://nuts-node:8081`         |
| `NUTS_ISSUER_DID`        | Yes (if using Nuts node) | DID to use as credential issuer   | `did:web:example.com:iam:org-a` |

### Enabling Nuts Node Integration

To enable Nuts node integration, set both environment variables:

```bash
export NUTS_NODE_INTERNAL_URL=http://nuts-node:8081
export NUTS_ISSUER_DID=did:web:example.com:iam:org-a
```

Or in docker-compose.yml:

```yaml
environment:
  NUTS_NODE_INTERNAL_URL: http://nuts-node:8081
  NUTS_ISSUER_DID: did:web:example.com:iam:org-a
```

### Using Local Signing (Default)

If `NUTS_NODE_INTERNAL_URL` is not set, the issuer will use local signing:

```bash
# Only ISSUER_HOSTNAME is needed for local signing
export ISSUER_HOSTNAME=localhost:3000
```

## How It Works

### Mode Detection

The issuer automatically detects which mode to use:

```typescript
// lib/nuts/client.ts
export function isNutsNodeEnabled(): boolean {
    return !!process.env.NUTS_NODE_INTERNAL_URL;
}
```

### Credential Issuance Flow

When issuing a credential (`POST /api/oidc4vci/credential`):

1. **Check configuration**: Is `NUTS_NODE_INTERNAL_URL` set?
2. **If yes** (Nuts node mode):
    - Validate that `NUTS_ISSUER_DID` is configured
    - Call Nuts node API: `POST {NUTS_NODE_INTERNAL_URL}/internal/vcr/v2/issuer/vc`
    - Return the signed credential from Nuts node
3. **If no** (local mode):
    - Generate/retrieve local Ed25519 key pair
    - Sign credential locally using jose library
    - Return the signed credential

### Issuer DID Selection

The issuer DID is selected based on configuration:

```typescript
// lib/utils.ts
export function getIssuerDid(req?: NextRequest): string {
    // If Nuts issuer DID is configured, use it
    const nutsIssuerDid = process.env.NUTS_ISSUER_DID;
    if (nutsIssuerDid) {
        return nutsIssuerDid;
    }

    // Otherwise, generate did:web from hostname
    const hostname = getIssuerHostname(req);
    return generateDidWeb(hostname);
}
```

## Implementation Details

### New Files

- `lib/nuts/client.ts`: Nuts node API client with credential issuance function

### Modified Files

- `lib/utils.ts`: Updated `getIssuerDid()` to support configurable issuer DID
- `app/api/oidc4vci/credential/route.ts`: Updated to support both local and Nuts node issuance
- `.env.example`: Added Nuts node configuration variables
- `docker-compose.yml`: Added commented-out Nuts node configuration
- `README.md`: Documented Nuts node integration feature
- `CLAUDE.md`: Added development notes about Nuts node integration

### API Endpoint Used

The issuer calls the Nuts node's VCR API:

```
POST {NUTS_NODE_INTERNAL_URL}/internal/vcr/v2/issuer/vc
Content-Type: application/json

{
  "type": "VektisOrgCredential",
  "issuer": "did:web:example.com:iam:org-a",
  "credentialSubject": {
    "id": "did:web:wallet.example.com",
    "organizationName": "Apotheek De Zonnehoek",
    "organizationType": "pharmacy",
    "agbCode": "06010713",
    "uraNumber": "32475534"
  },
  "@context": [
    "https://www.w3.org/2018/credentials/v1",
    "https://vc-issuer.example.com/contexts/vektis-org.jsonld"
  ],
  "expirationDate": "2025-12-01T12:00:00Z"
}
```

Response:

```json
{
  "credential": "eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCIsImtpZCI6ImRpZDp3ZWI6ZXhhbXBsZS5jb206aWFtOm9yZy1hI2tleS0xIn0..."
}
```

## Error Handling

### Validation Errors

If `NUTS_NODE_INTERNAL_URL` is set but `NUTS_ISSUER_DID` is not:

```json
{
  "error": "server_error",
  "error_description": "Failed to sign credential"
}
```

Console log:

```
[Credential] ERROR: Failed to sign credential: Error: NUTS_ISSUER_DID is required when using Nuts node for credential issuance
```

### Nuts Node API Errors

If the Nuts node API call fails:

```json
{
  "error": "server_error",
  "error_description": "Failed to sign credential"
}
```

Console log:

```
[NutsClient] ERROR: Failed to issue credential via Nuts node
[NutsClient] Status: 500
[NutsClient] Response: <error details from Nuts node>
```

## Testing

### Local Mode (Default)

```bash
# No special configuration needed
npm run dev

# Test credential issuance
# Follow the OID4VCI flow and check logs for:
# [Credential] Using Nuts node for issuance: false
# [Credential] Issuing with local signing
```

### Nuts Node Mode

```bash
# Configure Nuts node integration
export NUTS_NODE_INTERNAL_URL=http://localhost:8081
export NUTS_ISSUER_DID=did:web:example.com:iam:org-a

npm run dev

# Test credential issuance
# Follow the OID4VCI flow and check logs for:
# [Credential] Using Nuts node for issuance: true
# [Credential] Issuing via Nuts node: http://localhost:8081
# [NutsClient] Issuing credential via Nuts node: http://localhost:8081/internal/vcr/v2/issuer/vc
```

## Benefits

1. **Flexibility**: Choose between local signing and Nuts node issuance without code changes
2. **Integration**: Seamlessly integrate with existing Nuts node infrastructure
3. **Backward Compatible**: Existing deployments continue to work without any changes
4. **Configuration-Based**: Mode selection is entirely configuration-driven
5. **Clear Errors**: Helpful error messages when configuration is incomplete

## Future Enhancements

Potential improvements:

- Support for multiple credential types via Nuts node
- Caching of Nuts node responses
- Health check endpoint for Nuts node connectivity
- Automatic retry logic for transient Nuts node failures
- Support for custom Nuts node authentication (if needed)

