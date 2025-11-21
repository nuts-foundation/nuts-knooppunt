# Demo EHR

A simple single-page application (SPA) that demonstrates integration with the Nuts Knooppunt OIDC Provider.

## Features

- **OIDC Authentication** - Login using Knooppunt's OIDC Provider
- **User Dashboard** - Display authenticated user information
- **Simple & Fast** - Easy to run and develop
- **Foundation for Data Exchange** - Ready to integrate with Knooppunt's data exchange features

## Prerequisites

1. Nuts Knooppunt running on `http://localhost:8080`
2. Node.js (v14 or higher) and npm installed

## Configuration

The application is configured to use the following OIDC settings (see `src/authConfig.js`):

- **Authority**: `http://localhost:8080/auth` (public interface)
- **Client ID**: `demo-ehr`
- **Client Secret**: `demo-ehr-secret`
- **Redirect URI**: `http://localhost:3000/callback`

### Knooppunt Configuration

Add the following client configuration to your Knooppunt config file (`config/knooppunt.yml`):

```yaml
authn:
  clients:
    - id: "demo-ehr"
      secret: "demo-ehr-secret"
      redirecturls:
        - "http://localhost:3000/callback"
```

## Installation

```bash
cd demo-ehr
npm install
```

## Running the Application

### Option 1: Local Development

```bash
npm start
```

The application will start on `http://localhost:3000`.

### Option 2: Docker

```bash
# Build and run with Docker
docker build -t demo-ehr .
docker run -it -p 3000:3000 demo-ehr

# Or use Docker Compose
docker-compose up
```

See [README_DOCKER.md](README_DOCKER.md) for detailed Docker instructions.

## Usage

1. Start the Nuts Knooppunt server
2. Start the Demo EHR application
3. Navigate to `http://localhost:3000`
4. Click "Login with Knooppunt"
5. Complete the authentication flow
6. You'll be redirected back to the EHR dashboard

## New Patient Feature

You can create a new patient via the Patients Overview page:

1. Navigate to `/patients` (click View Patients on dashboard)
2. Click the "New Patient" button
3. Fill in:
   - BSN (optional, 9 digits)
   - Given name(s)
   - Family name
   - Prefix(es) (optional)
   - Birth date (YYYY-MM-DD)
   - Gender (male/female/other/unknown)
4. Click Create to POST a FHIR Patient resource to the configured FHIR server.

The new patient appears at the top of the list immediately after creation.

## Architecture

- **React** - UI framework
- **oidc-client-ts** - OIDC/OAuth2 client library
- **React Router** - Client-side routing

## Documentation

- [README_DOCKER.md](README_DOCKER.md) - Docker deployment guide
- [SETUP.md](SETUP.md) - Detailed setup instructions (if exists)
- [AUTHENTICATION_FLOW.md](AUTHENTICATION_FLOW.md) - Complete OIDC flow documentation (if exists)

## Future Enhancements

This demo application provides a foundation for:
- Patient record management
- Data exchange with other healthcare providers via Knooppunt
- Document reference search and retrieval
- FHIR resource integration
