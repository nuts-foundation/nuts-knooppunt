# Demo EHR Application

A demonstration Electronic Health Record (EHR) application for showcasing Dutch healthcare data exchange use cases, specifically focusing on BGZ (Basisgegevensset Zorg) exchange and eOverdracht (care handover) workflows.

## Overview

The Demo EHR app is a React-based web application that demonstrates how healthcare providers can exchange patient data using FHIR standards and the Nuts infrastructure. It implements two primary use cases:

1. **BGZ (Basisgegevensset Zorg) Exchange** - Sharing comprehensive patient health summaries
2. **eOverdracht** - Care handover between healthcare organizations

## Key Capabilities

### 1. BGZ Exchange Use Case

The BGZ exchange workflow enables healthcare organizations to request and share comprehensive patient health summaries. The application supports:

#### BGZ Workflow Task Creation
- Creates FHIR Worfklow Task resources conforming to the `Ta Notified Pull`: https://www.twiin.nl/tanp
- Defines comprehensive data queries covering all BGZ components:
  - Patient demographics and general practitioner information
  - Payment sources and coverage information
  - Treatment instructions and advance directives
  - Functional status assessments
  - Problem list and health concerns
  - Living situation and social history (drug use, alcohol, tobacco)
  - Allergies and intolerances
  - Medication (statements, agreements, administration)
  - Medical devices and aids
  - Immunization history
  - Vital signs (blood pressure, weight, height)
  - Laboratory results
  - Procedures and surgical history
  - Care plans
  - Medical records and diagnostic reports

#### BGZ Notification Task Creation
- Creates FHIR Notification Task resources conforming to the `Ta Notified Pull`: https://www.twiin.nl/tanp
- Supports two modes (as TA NP defines):
  - **With Workflow Task**: References an existing BGZ workflow task (via `basedOn` reference)
  - **Without Workflow Task**: Includes all FHIR query parameters directly in the notification

#### BGZ Data Generation
- Generates example BGZ bundles for demonstration purposes
- Supports customization of patient references
- Posts FHIR bundles to STU3 FHIR server

#### BGZ Visualization
- Query and display BGZ data from external organizations
- Integration with BGZ visualization services

### 2. eOverdracht Use Case

The eOverdracht workflow facilitates structured care handover between healthcare providers:

#### eOverdracht Task Creation
- Creates FHIR Task resources conforming to the `eOverdracht-Task` profile
- Captures:
  - Patient information
  - Requesting practitioner details
  - Receiving organization/department
  - Care handover documentation (Composition reference)

#### eOverdracht Task Management
- Query existing eOverdracht tasks by patient
- Filter tasks by patient references
- Display task status and metadata

#### Notification Workflow
- Send notifications to receiving party endpoints
- POST task resources to external organization endpoints

### 3. Organization Localization and Routing

The application leverages two key infrastructure components for discovering and routing to healthcare organizations:

#### mCSD (Mobile Care Services Directory)
- Query healthcare services and organizations from the mCSD directory
- Browse organization hierarchies (parent/child relationships via `partOf`)
- Retrieve organization details:
  - URA identifiers
  - Contact information (telecom)
  - Addresses
  - Organization types
  - Active status
- Find sub-organizations (departments)

#### mCSD Endpoint Discovery
- Discover endpoints for organizations
- Traverse organization hierarchy to find endpoints
- Filter endpoints by payload type:
  - `eOverdracht-notification` - For care handover notifications
  - `Twiin-TA-notification` - For BGZ notifications
- Support multiple endpoints per organization

#### NVI (Notified Pull Index)
- Search for DocumentReferences by patient BSN (Burgerservicenummer)
- Discover organizations that have data for a specific patient
- Build patient care networks by identifying custodian organizations
- Extract organization metadata:
  - URA identifiers from DocumentReference custodians
  - Organization names
  - Document counts per organization
  - Last document timestamps
- Tenant-based queries using `X-Tenant-ID` header with URA identifiers

### 4. Patient Management

- Browse and search patients
- View patient demographics
- Access patient identifiers (BSN)
- Query medication information:
  - Medication requests
  - Medication dispenses
- View patient care network organizations (via NVI)

### 5. Consent Management

- View and manage patient consents
- FHIR Consent resource support

## Data Sources

### FHIR Servers

The application interacts with multiple FHIR endpoints:

1. **FHIR R4 Server** (`config.fhirBaseURL`)
   - Primary patient data source
   - Patient demographics
   - Observations, conditions, procedures
   - Default: `http://localhost:7050/fhir/sunflower-patients`

2. **FHIR STU3 Server** (`config.fhirStu3BaseURL`)
   - BGZ and eOverdracht task management
   - Legacy FHIR STU3 resources
   - Default: `http://localhost:7060/fhir`

3. **mCSD Query Directory** (`config.mcsdQueryBaseURL`)
   - Organization directory
   - HealthcareService directory
   - Endpoint registry
   - Default: `http://localhost:8080/fhir`

### NVI (Notified Pull Index)

The NVI service provides a decentralized index of patient documents:

- **Endpoint**: `/api/knooppunt/nvi/DocumentReference`
- **Query Parameters**:
  - `patient:identifier` - Search by BSN identifier
  - `_count` - Result limit
- **Headers**:
  - `X-Tenant-ID` - Organization URA for tenant isolation
- **Response**: FHIR Bundle of DocumentReference resources with custodian organizations

### Example Data

The application includes example FHIR resources for testing:

- `bgz-example.json` - Sample BGZ bundle (STU3)
- `bzg-example-r4.json` - Sample BGZ bundle (R4)
- `workflow-task.json` - Sample BGZ workflow task
- `notification-task.json` - Sample notification task

## Architecture

### Frontend
- **Framework**: React 18.2
- **Routing**: React Router DOM 6.20
- **Authentication**: OIDC Client TS 3.0
- **Proxy**: HTTP Proxy Middleware 3.0

### API Modules

The application is organized into API client modules:

- `bgzApi.js` - BGZ data generation
- `bgzVerweijzingApi.js` - BGZ workflow and notification tasks
- `bgzVisualizationApi.js` - BGZ data visualization
- `eOverdrachtApi.js` - eOverdracht tasks and notifications
- `organizationApi.js` - mCSD organization queries
- `healthcareServiceApi.js` - mCSD healthcare service queries
- `nviApi.js` - NVI DocumentReference queries
- `patientApi.js` - Patient queries
- `medicationApi.js` - Medication queries
- `consentApi.js` - Consent queries
- `practitionerApi.js` - Practitioner queries
- `fhir.js` - FHIR utilities and headers

### Proxy Configuration

The app uses `setupProxy.js` to proxy requests:
- `/api/knooppunt/*` - Routes to Nuts node
- `/api/dynamic-proxy/*` - Dynamic proxy using `X-Target-URL` header

## Configuration

Application configuration is managed through environment variables. The application reads configuration from `config.js`, which consumes React environment variables.

### Environment Variables

Configure the following environment variables (see `docker-compose.yml` profile `demoehr`):

| Variable | Description | Default/Example |
|----------|-------------|-----------------|
| `REACT_APP_AUTHORITY` | OIDC authority endpoint for authentication | `http://localhost:8080/auth` |
| `REACT_APP_FHIR_BASE_URL` | FHIR R4 base URL for patient data | `https://server.fire.ly/R4` |
| `REACT_APP_FHIR_STU3_BASE_URL` | FHIR STU3 base URL for BGZ/eOverdracht tasks | `https://server.fire.ly/R3` |
| `REACT_APP_FHIR_MCSD_QUERY_BASE_URL` | mCSD Query Directory FHIR endpoint | `http://localhost:7050/fhir/knpt-mcsd-query` |
| `REACT_APP_ORGANIZATION_URA` | Organization URA identifier (optional) | - |
| `CHOKIDAR_USEPOLLING` | Enable polling for hot reload in Docker | `true` |

### Docker Compose Configuration

The demo-ehr service is available as a Docker Compose profile. Start with:

```bash
docker compose --profile demoehr up
```

The service configuration from `docker-compose.yml`:

```yaml
demo-ehr:
  build:
    context: ./demo-ehr
    dockerfile: Dockerfile
  image: demo-ehr:latest
  profiles:
    - demoehr
  ports:
    - "3000:3000"
  volumes:
    - ./demo-ehr/src:/app/src
    - ./demo-ehr/public:/app/public
  environment:
    - CHOKIDAR_USEPOLLING=true
    - REACT_APP_AUTHORITY=http://localhost:8080/auth
    - REACT_APP_FHIR_BASE_URL=https://server.fire.ly/R4
    - REACT_APP_FHIR_STU3_BASE_URL=https://server.fire.ly/R3
    - REACT_APP_FHIR_MCSD_QUERY_BASE_URL=http://localhost:7050/fhir/knpt-mcsd-query
```

### Local Development Configuration

For local development without Docker, create a `.env` file in the `demo-ehr` directory:

```bash
REACT_APP_AUTHORITY=http://localhost:8081
REACT_APP_FHIR_BASE_URL=http://localhost:7050/fhir/sunflower-patients
REACT_APP_FHIR_STU3_BASE_URL=http://localhost:7060/fhir
REACT_APP_FHIR_MCSD_QUERY_BASE_URL=http://localhost:7050/fhir/knpt-mcsd-query
```

### OIDC Configuration

OIDC settings are configured in `authConfig.js`:

- **Authority**: `http://localhost:8081` (or from `REACT_APP_AUTHORITY`)
- **Client ID**: `demo-ehr`
- **Client Secret**: `demo-ehr-secret`
- **Redirect URI**: `http://localhost:3000/callback`
- **Scopes**: `openid profile`

### Knooppunt Configuration

The demo-ehr application requires specific configuration in the Knooppunt server (`config/knooppunt.yml`). The following sections detail the required configuration changes:

#### NVI Configuration

Enable NVI (Notified Pull Index) with the FHIR base URL:

```yaml
nvi:
  baseurl: "http://localhost:7050/fhir/nvi"
```

#### mCSD Configuration

Configure mCSD directories for organization discovery and routing:

```yaml
mcsd:
  # Root directories to synchronize from
  admin:
    "lrza":
      fhirbaseurl: "https://knooppunt-test.nuts-services.nl/lrza/mcsd"

  # Local FHIR directory configuration
  query:
    fhirbaseurl: "http://localhost:7050/fhir/knpt-mcsd-query"
```

**Key Points:**
- `mcsd.admin` - Configures external mCSD directories to synchronize from (e.g., LRZA national registry)
- `mcsd.query.fhirbaseurl` - Local query endpoint for organization and endpoint lookups
- The query endpoint is used by the demo-ehr app via `REACT_APP_FHIR_MCSD_QUERY_BASE_URL`

#### mCSD Admin Configuration

Configure the admin interface FHIR endpoint:

```yaml
mcsdadmin:
  # Base URL for FHIR server used by admin interface
  fhirbaseurl: "http://localhost:7050/fhir/knpt-mcsd-admin"
```

#### OIDC Client Registration

Register the demo-ehr application as an OIDC client:

```yaml
authn:
  oidc:
    # ... existing configuration ...
    clients:
      # ... existing clients ...
      - id: "demo-ehr"
        secret: "demo-ehr-secret"
        redirecturls:
          - "http://localhost:3000/callback"
```

**Important:** The client ID, secret, and redirect URL must match the values in `demo-ehr/src/authConfig.js`.

#### Complete Configuration Example

Here's a complete example of the required sections in `config/knooppunt.yml`:

```yaml
strictmode: false

nvi:
  baseurl: "http://localhost:7050/fhir/nvi"

mcsd:
  admin:
    "lrza":
      fhirbaseurl: "https://knooppunt-test.nuts-services.nl/lrza/mcsd"

  query:
    fhirbaseurl: "http://localhost:7050/fhir/knpt-mcsd-query"

  exclude_admin_directories:
    - "http://localhost:7050/fhir/knpt-mcsd-admin"

mcsdadmin:
  fhirbaseurl: "http://localhost:7050/fhir/knpt-mcsd-admin"

authn:
  oidc:
    clients:
      - id: "demo-ehr"
        secret: "demo-ehr-secret"
        redirecturls:
          - "http://localhost:3000/callback"
```

## FHIR Profiles and Standards

### BGZ (Basisgegevensset Zorg)
- **Profile**: `http://nictiz.nl/fhir/StructureDefinition/BgZ-verwijzing-Task`
- **Standard**: Nictiz BgZ information standard
- **Purpose**: Exchange comprehensive patient summaries between healthcare providers

### eOverdracht
- **Profile**: `http://nictiz.nl/fhir/StructureDefinition/eOverdracht-Task`
- **Standard**: Nictiz eOverdracht information standard
- **Purpose**: Structured care handover with nursing transfer reports

### Identifiers
- **BSN**: `http://fhir.nl/fhir/NamingSystem/bsn` - Dutch citizen service number
- **URA**: `http://fhir.nl/fhir/NamingSystem/ura` - Unique healthcare provider identification

### Task Codes
- **BGZ Referral**: SNOMED CT 3457005 - "verwijzen van patiënt"
- **eOverdracht**: SNOMED CT 308292007 - "Overdracht van zorg"
- **Pull Notification**: `http://fhir.nl/fhir/NamingSystem/TaskCode|pull-notification`