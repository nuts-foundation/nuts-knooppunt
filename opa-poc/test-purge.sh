#!/bin/bash

echo "=== Testing FHIR Purge Functionality ==="
echo ""

# 1. Create a test patient
echo "1. Creating test patient..."
curl -s -X POST \
  -H "Content-Type: application/fhir+json" \
  -d '{
    "resourceType": "Patient",
    "id": "test-patient-999",
    "name": [{"family": "TestPurge"}]
  }' \
  "http://localhost:7623/fhir/Patient/test-patient-999" > /dev/null

echo "   Patient created"
sleep 1

# 2. Verify patient exists
echo "2. Verifying patient exists..."
result=$(curl -s "http://localhost:7623/fhir/Patient/test-patient-999" 2>&1 | grep -c '"id":"test-patient-999"' || echo "0")
if [ "$result" -gt 0 ]; then
  echo "   ✅ Patient found"
else
  echo "   ❌ Patient NOT found"
fi

# 3. Run purge
echo "3. Running FHIR purge..."
curl -s -X POST \
  "http://localhost:7623/fhir/\$expunge" \
  -H "Content-Type: application/fhir+json" \
  -d '{
    "resourceType": "Parameters",
    "parameter": [
      {
        "name": "expungeEverything",
        "valueBoolean": true
      }
    ]
  }' > /dev/null 2>&1

sleep 1
echo "   Purge completed"

# 4. Verify patient is gone
echo "4. Verifying patient is purged..."
status=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:7623/fhir/Patient/test-patient-999" 2>&1)
if [ "$status" = "404" ] || [ "$status" = "410" ]; then
  echo "   ✅ Patient successfully purged (status: $status)"
else
  echo "   ❌ Patient still exists (status: $status)"
fi

echo ""
echo "=== Done ==="

