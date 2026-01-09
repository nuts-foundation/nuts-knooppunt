#!/bin/bash

echo "=== Debugging FHIR Resource Fetch ==="

cd /Users/reinkrul/nuts-knooppunt/opa-poc/eOverdracht-sender-fhirsearch

# Create a test input
temp_input=$(mktemp)
jq --arg fhir_url "http://localhost:7623/fhir" \
    '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' \
    ok-with-category.json > "$temp_input"

echo "Input context.fhir_base_url:"
jq -r '.context.fhir_base_url' "$temp_input"
echo ""

echo "Input resource.type:"
jq -r '.resource.type' "$temp_input"
echo ""

echo "Input resource.properties.id:"
jq -r '.resource.properties.id' "$temp_input"
echo ""

echo "Expected URL:"
echo "http://localhost:7623/fhir/Observation/obs-12345"
echo ""

echo "Testing URL directly with curl:"
curl -s http://localhost:7623/fhir/Observation/obs-12345 | jq -r '.resourceType, .id, .code.coding[0].system, .code.coding[0].code'
echo ""

echo "Testing _fetch_fhir_resource in OPA:"
opa eval -i "$temp_input" \
    -d ../common.rego \
    -d policy.rego \
    'data.eoverdracht.sender._fetch_fhir_resource(input.resource.type, input.resource.properties.id)' \
    --format pretty 2>&1

rm -f "$temp_input"

