# Demo Dezi Client

Reference implementation for authenticating with Dezi (Dutch healthcare OIDC provider).

## Quick Start

```bash
go run main.go
```

## Configuration

Via environment variables (or `.env` file):

```bash
DEZI_AUTHORITY=https://acceptatie.auth.dezi.nl
DEZI_CLIENT_ID=212bd7bd-bb63-487b-81b2-00079716072d
DEZI_REDIRECT_URI=http://localhost:8090/callback
SERVER_PORT=8090
FRONTEND_BASE_URL=http://localhost:3000
```

## Endpoints

- `GET /login?return_url=...` - Start login
- `GET /callback` - OAuth callback
- `GET /userinfo` - Get user info (authenticated)
- `GET /logout` - Logout

## What It Does

Implements OIDC Authorization Code Flow with PKCE to connect demo-ehr to Dezi:

1. Generates PKCE challenge/verifier
2. Redirects to Dezi for authentication
3. Exchanges code for access token
4. Fetches and parses userinfo (JWT format)
5. Extracts verklaring (healthcare worker declaration)

## Integration with demo-ehr

Set `REACT_APP_AUTH_BASE_URL=http://localhost:8090` in demo-ehr's environment.

## Running

### Development
```bash
go run main.go
```

### Build and run
```bash
go build -o demo-dezi-client main.go
./demo-dezi-client
```

### Docker
```bash
docker-compose up
```

## Implementation Notes

- **Userinfo format**: Dezi acceptatie returns signed JWT (3 parts), not encrypted JWE (5 parts). Code handles both.
- **Sessions**: Stored in-memory using state as key. Lost on restart.
- **PKCE**: Uses S256 challenge method as required by Dezi spec.
- **Logging**: Logs ID token, userinfo envelope, and decoded verklaring for debugging.


Based on Dezi spec v0.7.

