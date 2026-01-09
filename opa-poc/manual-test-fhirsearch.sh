#!/bin/bash
set -e

echo "=== Manual Test of eOverdracht-sender-fhirsearch ==="
echo ""

# Check if FHIR server is running
echo "1. Checking FHIR server status..."
if curl -sf http://localhost:7623/fhir/metadata > /dev/null 2>&1; then
    echo "   ✅ FHIR server is running"
else
    echo "   ❌ FHIR server is NOT running"
    echo "   Starting FHIR server..."
    docker run -d --name opa-test-fhir-server -p 7623:8080 hapiproject/hapi:v8.6.0-1
    echo "   Waiting for server to start..."
    sleep 20
fi
echo ""

# Load test data
echo "2. Loading test data..."
cd /Users/reinkrul/nuts-knooppunt/opa-poc/eOverdracht-sender-fhirsearch
response=$(curl -s -X POST \
    -H "Content-Type: application/fhir+json" \
    -d @ok-with-category.testdata.json \
    http://localhost:7623/fhir)

if echo "$response" | jq -e '.type == "transaction-response"' > /dev/null 2>&1; then
    echo "   ✅ Transaction succeeded"
else
    echo "   ❌ Transaction may have failed"
    echo "$response" | jq '.' 2>&1 | head -20
fi
echo ""

# Wait for indexing
echo "3. Waiting for FHIR indexing..."
sleep 2
echo ""

# Verify resource was created
echo "4. Checking if resource exists..."
resource=$(curl -s http://localhost:7623/fhir/Observation/obs-12345)
if echo "$resource" | jq -e '.id == "obs-12345"' > /dev/null 2>&1; then
    echo "   ✅ Resource exists"
    echo "   Resource code: $(echo "$resource" | jq -r '.code.coding[0].code')"
else
    echo "   ❌ Resource NOT found"
fi
echo ""

# Test OPA policy
echo "5. Testing OPA policy..."
temp_input=$(mktemp)
jq --arg fhir_url "http://localhost:7623/fhir" \
    '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' \
    ok-with-category.json > "$temp_input"

result=$(opa eval -i "$temp_input" \
    -d ../common.rego \
    -d policy.rego \
    "data.eoverdracht.sender.allow" \
    --format raw 2>&1)

rm -f "$temp_input"

echo "   Result: $result"
if [ "$result" == "true" ]; then
    echo "   ✅ TEST PASSED"
else
    echo "   ❌ TEST FAILED"
    echo ""
    echo "6. Debugging..."
    # Check each condition
    echo "   Testing resource_category_allowed..."
    temp_input2=$(mktemp)
    jq --arg fhir_url "http://localhost:7623/fhir" \
        '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' \
        ok-with-category.json > "$temp_input2"

    opa eval -i "$temp_input2" \
        -d ../common.rego \
        -d policy.rego \
        "data.eoverdracht.sender.resource_category_allowed" \
        --format pretty 2>&1 | head -5

    rm -f "$temp_input2"
fi

echo ""
echo "=== Done ==="

