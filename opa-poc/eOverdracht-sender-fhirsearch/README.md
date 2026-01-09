# eOverdracht Sender with FHIR Search Authorization

This policy demonstrates FHIR search-based authorization where access to resources is determined by querying the FHIR server to verify that resources match specific criteria.

## Policy Rules

- **Observation resources**: Must have code `heart-measurement`
- **Condition resources**: Must have code `heartfailure`

## Test Cases

### Positive Tests

1. **ok-with-category.json**
   - Resource: Observation with ID `obs-12345`
   - FHIR data: Observation with code `heart-measurement`
   - Expected: Allow access

2. **ok-condition-heartfailure.json**
   - Resource: Condition with ID `condition-12345`
   - FHIR data: Condition with code `heartfailure`
   - Expected: Allow access

### Negative Tests

3. **nok-wrong-code.json**
   - Resource: Observation with ID `obs-wrong-code`
   - FHIR data: Observation with code `blood-pressure` (wrong code)
   - Expected: Deny access (resource category not allowed)

4. **nok-resource-not-found.json**
   - Resource: Observation with ID `obs-nonexistent`
   - FHIR data: No resource created
   - Expected: Deny access (resource not found in FHIR)

## FHIR Test Data Files

Each test case has a corresponding `.testdata.json` file that contains a FHIR transaction bundle to initialize the FHIR server with test data before running the test.

### File Naming Convention

- `<test-name>.json` - OPA policy test input
- `<test-name>.expected.json` - Expected OPA evaluation output
- `<test-name>.testdata.json` - FHIR transaction to initialize test data

### How It Works

1. Before each test runs, the test runner checks for a `.testdata.json` file
2. If found, it POSTs the transaction bundle to the FHIR server at `http://localhost:7623/fhir`
3. The FHIR server creates/updates the resources specified in the transaction
4. The OPA policy is evaluated, which makes FHIR search API calls
5. The FHIR server returns results based on the test data that was just loaded

### Example FHIR Transaction

```json
{
  "resourceType": "Bundle",
  "type": "transaction",
  "entry": [
    {
      "fullUrl": "urn:uuid:obs-12345",
      "request": {
        "method": "PUT",
        "url": "Observation/obs-12345"
      },
      "resource": {
        "resourceType": "Observation",
        "id": "obs-12345",
        "code": {
          "coding": [
            {
              "code": "heart-measurement"
            }
          ]
        }
      }
    }
  ]
}
```

## Running Tests

Simply run the test script:

```bash
cd /Users/reinkrul/nuts-knooppunt/opa-poc
./run-all-tests.sh
```

The script will:
1. Start/reuse HAPI FHIR server on port 7623
2. For each test, load FHIR test data via transaction
3. Run OPA policy evaluation (which queries FHIR)
4. Compare actual vs expected results

## Policy Implementation Details

### FHIR Search Optimization

The policy uses `_summary=count` in FHIR search queries for optimal performance:

```
GET {base}/Observation?_summary=count&_id=obs-12345&code=heart-measurement
```

**Benefits:**
- **Faster**: Only returns the count, not the full resource data
- **Efficient**: Reduces network bandwidth and processing time
- **Sufficient**: For authorization, we only need to know if a matching resource exists

**Response Structure:**
```json
{
  "resourceType": "Bundle",
  "type": "searchset",
  "total": 1
}
```

The policy checks `response.body.total > 0` to determine if access should be allowed.
