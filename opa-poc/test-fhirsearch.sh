#!/bin/bash

# Quick test for eOverdracht-sender-fhirsearch

set -e

echo "Testing eOverdracht-sender-fhirsearch policy..."
echo ""

# Check if FHIR server is running
if ! docker ps | grep -q opa-test-fhir-server; then
    echo "Starting FHIR server..."
    docker run -d --name opa-test-fhir-server -p 7623:8080 hapiproject/hapi:v8.6.0-1
    echo "Waiting for FHIR server to start..."
    sleep 20
fi

cd /Users/reinkrul/nuts-knooppunt/opa-poc/eOverdracht-sender-fhirsearch

# Test 1: ok-with-category
echo "Test 1: ok-with-category"
echo "Loading testdata..."
curl -s -X POST \
    -H "Content-Type: application/fhir+json" \
    -d @ok-with-category.testdata.json \
    http://localhost:7623/fhir > /dev/null

sleep 2

echo "Running OPA evaluation..."
temp_input=$(mktemp)
jq --arg fhir_url "http://localhost:7623/fhir" \
    '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' ok-with-category.json > "$temp_input"

result=$(opa eval -i "$temp_input" -d ../common.rego -d policy.rego "data.eoverdracht.sender.allow" --format raw 2>&1)
rm -f "$temp_input"

echo "Result: $result"
echo "Expected: true"

if [ "$result" == "true" ]; then
    echo "✅ PASS"
else
    echo "❌ FAIL"
    # Debug: check FHIR search
    echo "Debug: Testing FHIR search..."
    curl -s "http://localhost:7623/fhir/Observation?_summary=count&_id=obs-12345&code=heart-measurement" | jq '.total'
fi

echo ""
echo "Done"

