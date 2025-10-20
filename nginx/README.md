# NGINX mTLS Proxy for MITZ

This directory contains an NGINX reverse proxy configuration that handles mutual TLS (mTLS) authentication with the MITZ API, so the Knooppunt application doesn't need to manage client certificates directly.

This is only here for testing/development purposes (and hackaton), so that each vendor does not necessarily need to do this themselves (get certificate, get whitelisted with mitz, ...).

## Purpose

The MITZ API requires mutual TLS authentication, which means:
1. The MITZ server presents its TLS certificate to prove its identity
2. The client must present a valid client certificate that MITZ trusts

Instead of configuring mTLS directly in the Go application, we use NGINX as a proxy that:
- Accepts plain HTTP connections from Knooppunt on `localhost:8080`
- Handles mTLS authentication with the MITZ API server
- Forwards requests to `https://tst-api.mijn-mitz.nl:443`
- Accepts MITZ notification callbacks at `/mitz/notify` and `/mitz/notify/Consent` (returns 201 without processing)

## Architecture

### Outbound Requests (Knooppunt → MITZ)

```
┌─────────────┐         HTTP           ┌──────────┐       HTTPS + mTLS      ┌──────────────┐
│             │  ───────────────────►  │          │  ─────────────────────► │              │
│  Knooppunt  │   localhost:8080       │  NGINX   │   tst-api.mijn-mitz.nl  │  MITZ API    │
│  (Go App)   │                        │  Proxy   │                         │              │
│             │  ◄─────────────────────│          │  ◄───────────────────── │              │
└─────────────┘                        └──────────┘                         └──────────────┘
```

When Knooppunt makes API calls to MITZ (e.g., creating subscriptions):
1. Knooppunt sends HTTP request to `http://localhost:8080`
2. NGINX forwards to MITZ with mTLS authentication
3. MITZ response is proxied back to Knooppunt

### Inbound Notifications (MITZ → Proxy)

```
┌─────────────┐                        ┌──────────┐                         ┌──────────────┐
│             │                        │          │  ◄───────────────────── │              │
│  Knooppunt  │                        │  NGINX   │   POST /mitz/notify     │  MITZ API    │
│  (Go App)   │                        │  Proxy   │   (consent change)      │              │
│             │                        │          │  ─────────────────────► │              │
│             │                        │          │   201 Created           │              │
└─────────────┘                        └──────────┘                         └──────────────┘
                                            │
                                            └─ Returns 201 immediately
                                               (does NOT forward to Knooppunt)
```

When MITZ sends consent change notifications:
1. MITZ sends HTTP POST to the proxy's public endpoint
2. NGINX receives the notification at `/mitz/notify` or `/mitz/notify/Consent`
3. NGINX immediately returns 201 Created
4. Notification is **not** forwarded to Knooppunt or processed


## Configuration

### NGINX Configuration

The `nginx.conf` file configures:
- **Upstream**: `tst-api.mijn-mitz.nl:443` (MITZ test API)
- **Notification Endpoints**: `/mitz/notify` and `/mitz/notify/Consent` return 201 (not forwarded to MITZ)
- **mTLS Settings**:
  - `proxy_ssl_certificate`: Path to client certificate
  - `proxy_ssl_certificate_key`: Path to client private key
  - `proxy_ssl_verify off`: Disables server certificate verification

### Knooppunt Configuration

Update `config/knooppunt.yml` to point to the NGINX proxy instead of directly to MITZ:

```yaml
mitz:
  # Point to local NGINX proxy instead of MITZ directly
  mitzbase: "http://localhost:8080/tst-us/mitz"

  # No TLS configuration needed - NGINX handles it
  # tls_cert_file: ""
  # tls_key_file: ""
  # tls_key_password: ""
```

## Certificate Setup

### Required Certificates

Place the following files in the `certs/` directory:

1. **`client.crt`** - Client certificate in PEM format
   - This certificate must be whitelisted by the MITZ team
   - Extract from `.p12` file if needed (see below)

2. **`client.key`** - Client private key in PEM format
   - Must match the client certificate
   - Keep this file secure (permissions: 600)

3. **`upstream-ca.pem`** (optional) - CA certificate for MITZ server verification
   - Only needed if `proxy_ssl_verify on` is set
   - Contains the CA that signed the MITZ server certificate

### Converting PKCS#12 (.p12) to PEM

If you have a `.p12` file (like `vitaly-acc-1.oehp.nl.p12`), extract the certificate and key:

```bash
# Extract certificate
openssl pkcs12 -in vitaly-acc-1.oehp.nl.p12 -clcerts -nokeys -out certs/client.crt

# Extract private key
openssl pkcs12 -in vitaly-acc-1.oehp.nl.p12 -nocerts -nodes -out certs/client.key

# Set secure permissions
chmod 600 certs/client.key
chmod 644 certs/client.crt
```

You'll be prompted for the `.p12` password during extraction.


## Running the Proxy

### Using Docker

```bash
# From the nginx directory
docker run -d \
  --name mitz-proxy \
  -p 8087:8080 \
  -v $(pwd)/nginx.conf:/etc/nginx/nginx.conf:ro \
  -v $(pwd)/certs:/etc/nginx/certs:ro \
  nginx:alpine

# View logs
docker logs -f mitz-proxy

# Restart after certificate changes
docker restart mitz-proxy
```

### Using Docker Compose

Create a `docker-compose.yml` in the project root:

```yaml
version: '3.8'

services:
  mitz-proxy:
    image: nginx:alpine
    ports:
      - "8080:8080"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/certs:/etc/nginx/certs:ro
```

Then run:
```bash
docker-compose up -d mitz-proxy
```


## Notification Endpoints

The proxy handles MITZ notification callbacks directly without forwarding them to Knooppunt:

### Endpoints

- **`POST /mitz/notify`** - Generic consent notification callback
- **`POST /mitz/notify/Consent`** - Consent-specific notification callback

### Behavior

When MITZ sends a notification to these endpoints:
1. NGINX receives the notification
2. Returns HTTP 201 Created immediately
3. Does **not** forward the notification to Knooppunt
4. Does **not** process or store the notification payload

### Why Handle Here?

For development/testing purposes, we:
- Acknowledge notifications from MITZ without implementing full processing logic
- Avoid the complexity of notification handling in the application
- Avoid the complexity of each vendor having to arrange endpoint whitelisting with Mitz


## Prerequisites

### MITZ Team Whitelist

Before the proxy will work, the MITZ team must whitelist:

1. **Client Certificate**: The certificate in `certs/client.crt` must be registered with MITZ
   - Contact MITZ support to register your certificate
   - Provide the certificate's thumbprint and send it to `testmanagement@mijnmitz.nl`

2. **Notification Endpoint**: The proxy's publicly accessible endpoint must be whitelisted
   - For development: `http://your-public-ip:8087/mitz/notify`
   - The proxy handles these notifications and returns 201 (acknowledgment only, no processing)

