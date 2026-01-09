#!/bin/bash

# Test HAPI FHIR server responses

echo "=== Testing HAPI FHIR Server ==="
echo ""

# 1. Load test data
echo "1. Loading test observation..."
curl -s -X POST \
    -H "Content-Type: application/fhir+json" \
    -d @/Users/reinkrul/nuts-knooppunt/opa-poc/eOverdracht-sender-fhirsearch/ok-with-category.testdata.json \
    http://localhost:7623/fhir > /tmp/fhir-load-response.json

echo "Transaction response:"
cat /tmp/fhir-load-response.json | jq -r '.type, .entry[0].response.status' 2>/dev/null || echo "Error loading"
echo ""

# Wait a bit
sleep 1

# 2. Test direct resource GET
echo "2. Testing direct resource GET..."
curl -s "http://localhost:7623/fhir/Observation/obs-12345" | jq -r '.resourceType, .id, .code.coding[0].code' 2>/dev/null || echo "Resource not found"
echo ""

# 3. Test search with _id only
echo "3. Testing search with _id only..."
curl -s "http://localhost:7623/fhir/Observation?_id=obs-12345" | jq -r '.total, .entry[0].resource.id' 2>/dev/null || echo "Search failed"
echo ""

# 4. Test search with _id and code
echo "4. Testing search with _id and code..."
curl -s "http://localhost:7623/fhir/Observation?_id=obs-12345&code=heart-measurement" | jq -r '.total' 2>/dev/null || echo "Search failed"
echo ""

# 5. Test search with _summary=count
echo "5. Testing search with _summary=count..."
curl -s "http://localhost:7623/fhir/Observation?_summary=count&_id=obs-12345&code=heart-measurement" | jq '.' 2>/dev/null || echo "Search failed"
echo ""

echo "=== Done ==="

