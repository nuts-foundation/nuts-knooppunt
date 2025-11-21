# Demo EHR - Patients Feature

## Overview

The Demo EHR now includes a Patients Overview page that displays patient information from the FHIR server.

## Features

- **Patient List**: View all patients from the sunflower-patients FHIR endpoint
- **BSN Display**: Shows Dutch citizen service number (BSN) for each patient
- **Patient Details**: Name, gender, birth date, and calculated age
- **Search**: Filter patients by name or BSN
- **Real-time Data**: Fetches data directly from FHIR server

## How It Works

### FHIR Integration

The application connects to the FHIR server at:
```
http://localhost:7050/fhir/sunflower-patients
```

### Data Extraction

The FHIR API client (`src/api/fhirApi.js`) handles:

1. **Fetching Patients**: `GET /Patient` returns a Bundle of patient resources
2. **BSN Extraction**: Looks for identifiers with system:
   - `http://fhir.nl/fhir/NamingSystem/bsn`
   - `urn:oid:2.16.840.1.113883.2.4.6.3`
3. **Name Formatting**: Extracts official name with prefix, given names, and family name
4. **Demographics**: Birth date, gender, and age calculation

### UI Components

- **PatientsPage** (`src/pages/PatientsPage.js`): Main patients overview page
  - Patient table with BSN, name, gender, birth date, and age
  - Search functionality
  - Loading and error states
  - Authentication check

- **FHIR API** (`src/api/fhirApi.js`): Client for FHIR operations
  - `getPatients()`: Fetch all patients
  - `getPatient(id)`: Fetch single patient
  - Helper functions for BSN, name, birth date, gender

## Usage

1. **Start the stack**:
   ```bash
   docker-compose up
   ```

2. **Log in** to the Demo EHR at `http://localhost:3000`

3. **View Patients**: Click "View Patients" on the dashboard or navigate to `/patients`

4. **Search**: Type a name or BSN in the search box to filter

## Configuration

The FHIR base URL can be configured via environment variable:

```yaml
# In docker-compose.yml
environment:
  - REACT_APP_FHIR_BASE_URL=http://localhost:7050/fhir/sunflower-patients
```

Or in a `.env` file:
```
REACT_APP_FHIR_BASE_URL=http://localhost:7050/fhir/sunflower-patients
```

## FHIR Resource Structure

Expected Patient resource structure:

```json
{
  "resourceType": "Patient",
  "id": "example-123",
  "identifier": [
    {
      "system": "http://fhir.nl/fhir/NamingSystem/bsn",
      "value": "123456789"
    }
  ],
  "name": [
    {
      "use": "official",
      "family": "Doe",
      "given": ["John"],
      "prefix": ["Mr."]
    }
  ],
  "gender": "male",
  "birthDate": "1990-01-15"
}
```

## API Methods

### fhirApi.getPatients()
Fetches all patients from the FHIR server.

**Returns**: `Promise<Array<Patient>>`

### fhirApi.getPatient(id)
Fetches a single patient by ID.

**Parameters**:
- `id` (string): Patient ID

**Returns**: `Promise<Patient>`

### fhirApi.getBSN(patient)
Extracts BSN from patient identifiers.

**Parameters**:
- `patient` (Patient): FHIR Patient resource

**Returns**: `string | null`

### fhirApi.getPatientName(patient)
Formats patient name from FHIR name structure.

**Parameters**:
- `patient` (Patient): FHIR Patient resource

**Returns**: `string`

## Error Handling

The application handles:
- Network errors (FHIR server unavailable)
- Invalid responses (non-Bundle responses)
- Missing data (no BSN, no name, etc.)
- Authentication required

## Styling

Patient table styles are in `App.css`:
- Responsive design (mobile-friendly)
- Hover effects on table rows
- Color-coded badges for BSN
- Gender icons (♂️ ♀️)
- Search box with focus states

## Future Enhancements

Potential additions:
- Patient detail view
- Edit patient information
- Add new patients
- Filter by gender, age range
- Sort by column
- Pagination for large datasets
- Export to CSV
- Integration with other FHIR resources (Observations, Conditions, etc.)

## Troubleshooting

### "Failed to fetch patients"

**Check**:
1. FHIR server is running: `curl http://localhost:7050/fhir/sunflower-patients/Patient`
2. CORS is enabled on FHIR server
3. Network connectivity in Docker

### "No patients found"

**Check**:
1. Patients exist in the FHIR server
2. Correct tenant/partition is being queried
3. FHIR endpoint URL is correct

### Authentication Required

Make sure you're logged in via the Knooppunt OIDC provider before accessing `/patients`.

