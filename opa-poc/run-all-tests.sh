#!/bin/bash

# Enhanced OPA Policy Test Runner with Expected Output Verification
# Run from the opa-poc directory
# This script verifies both the 'allow' decision and 'deny_reason' output

set -e

echo "==========================================="
echo "OPA Policy Test Runner (Enhanced)"
echo "==========================================="
echo ""

# Check if opa is available
if ! command -v opa &> /dev/null; then
    echo "❌ Error: opa CLI not found!"
    echo "Please install it: brew install opa"
    exit 1
fi

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo "❌ Error: jq CLI not found!"
    echo "Please install it: brew install jq"
    exit 1
fi

echo "Using OPA version:"
opa version | head -1
echo ""

# Start HAPI FHIR server for testing
echo "==========================================="
echo "HAPI FHIR Server Setup"
echo "==========================================="
FHIR_CONTAINER_NAME="opa-test-fhir-server"

# Check if container already exists and is running
if docker ps --format '{{.Names}}' | grep -q "^${FHIR_CONTAINER_NAME}$"; then
  echo "FHIR server container already running, reusing it..."
  FHIR_CONTAINER_EXISTED=true
elif docker ps -a --format '{{.Names}}' | grep -q "^${FHIR_CONTAINER_NAME}$"; then
  echo "FHIR server container exists but is stopped, starting it..."
  docker start "$FHIR_CONTAINER_NAME"
  FHIR_CONTAINER_EXISTED=true
else
  echo "Starting new HAPI FHIR server on port 7623..."
  docker run -d \
    --name "$FHIR_CONTAINER_NAME" \
    -p 7623:8080 \
    -e hapi.fhir.default_encoding=json \
    -e hapi.fhir.expunge_enabled=true \
    -e hapi.fhir.allow_multiple_delete=true \
    hapiproject/hapi:v8.6.0-1
  FHIR_CONTAINER_EXISTED=false
fi

# Wait for FHIR server to be ready
echo -n "Waiting for FHIR server to be ready"
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
  if curl -s http://localhost:7623/fhir/metadata > /dev/null 2>&1; then
    echo " ✅ Ready!"
    break
  fi
  echo -n "."
  sleep 2
  ((attempt++))
done

if [ $attempt -eq $max_attempts ]; then
  echo " ❌ Failed to start!"
  if [ "$FHIR_CONTAINER_EXISTED" = false ]; then
    docker rm -f "$FHIR_CONTAINER_NAME" 2>/dev/null || true
  fi
  exit 1
fi

echo ""

# Function to purge all data from FHIR server using system-wide $expunge
purge_fhir_server() {
    # Use HAPI FHIR's system-wide $expunge operation to delete all data
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
        }' > /dev/null 2>&1 || true

    # Small delay to ensure expunge completes
    sleep 0.5
}

# Counters
total_tests=0
total_passed=0
total_failed=0
total_errors=0

# Find all directories containing policy.rego files
policy_dirs=$(find . -maxdepth 2 -name "policy.rego" -exec dirname {} \; | sort)

if [ -z "$policy_dirs" ]; then
    echo "❌ No policy.rego files found in subdirectories!"
    exit 1
fi

# Test each policy directory
for policy_dir in $policy_dirs; do
    policy_name=$(basename "$policy_dir")
    echo "==========================================="
    echo "Testing: $policy_name"
    echo "==========================================="

    # Find all JSON input files in the directory (excluding .expected.json and .testdata.json files)
    json_files=$(find "$policy_dir" -maxdepth 1 -name "*.json" -type f ! -name "*.expected.json" ! -name "*.testdata.json" | sort)

    if [ -z "$json_files" ]; then
        echo "⚠️  No test input files found in $policy_dir"
        echo ""
        continue
    fi

    # Read the package name from the policy file
    package_name=$(grep -m 1 "^package" "$policy_dir/policy.rego" | awk '{print $2}')

    if [ -z "$package_name" ]; then
        echo "❌ Could not determine package name from $policy_dir/policy.rego"
        ((total_errors++))
        echo ""
        continue
    fi

    # Test each JSON file
    for input_file in $json_files; do
        filename=$(basename "$input_file")
        echo -n "  Testing $filename ... "
        ((total_tests++))

        # Check for FHIR transaction file to initialize test data
        fhir_transaction_file="${input_file%.json}.testdata.json"
        if [ -f "$fhir_transaction_file" ]; then
            # Purge FHIR server to ensure clean state
            purge_fhir_server

            # Load test data into FHIR server
            fhir_response=$(curl -s -X POST \
                -H "Content-Type: application/fhir+json" \
                -d @"$fhir_transaction_file" \
                http://localhost:7623/fhir 2>&1)
            # Wait for FHIR server to index the resources
            sleep 2
        fi

        # Check if there's an expected output file
        expected_file="${input_file%.json}.expected.json"

        if [ -f "$expected_file" ]; then
            # Enhanced verification with expected output

            # Merge FHIR base URL into input JSON (create context if it doesn't exist)
            temp_input=$(mktemp)
            jq --arg fhir_url "http://localhost:7623/fhir" \
                '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' "$input_file" > "$temp_input"

            # Run OPA evaluation and get full output
            result=$(opa eval -i "$temp_input" -d "common.rego" -d "$policy_dir/policy.rego" "data.$package_name" --format pretty 2>&1)

            # Clean up temp file
            rm -f "$temp_input"

            # Check if evaluation failed
            if echo "$result" | grep -q "error occurred"; then
                echo "❌ ERROR (evaluation failed)"
                echo "     $result"
                ((total_errors++))
                ((total_failed++))
                continue
            fi

            # Extract allow value
            actual_allow=$(echo "$result" | jq -r '.allow // "undefined"' 2>/dev/null || echo "ERROR")
            expected_allow=$(jq -r '.allow // "undefined"' "$expected_file" 2>/dev/null || echo "ERROR")

            # Extract deny_reason if present
            actual_deny_reason=$(echo "$result" | jq -r '.deny_reason // null' 2>/dev/null || echo "null")
            expected_deny_reason=$(jq -r '.deny_reason // null' "$expected_file" 2>/dev/null || echo "null")

            # Verify results
            allow_match=true
            deny_reason_match=true

            if [ "$actual_allow" != "$expected_allow" ]; then
                allow_match=false
            fi

            # Only check deny_reason if expected file specifies one
            if [ "$expected_deny_reason" != "null" ] && [ "$actual_deny_reason" != "$expected_deny_reason" ]; then
                deny_reason_match=false
            fi

            if [ "$allow_match" = true ] && [ "$deny_reason_match" = true ]; then
                echo "✅ PASS"
                if [ "$expected_deny_reason" != "null" ]; then
                    echo "     allow=$actual_allow, deny_reason=\"$actual_deny_reason\""
                else
                    echo "     allow=$actual_allow"
                fi
                ((total_passed++))
            else
                echo "❌ FAIL"
                if [ "$allow_match" = false ]; then
                    echo "     allow: expected=$expected_allow, actual=$actual_allow"
                fi
                if [ "$deny_reason_match" = false ]; then
                    echo "     deny_reason: expected=\"$expected_deny_reason\""
                    echo "                  actual=\"$actual_deny_reason\""
                fi
                ((total_failed++))
            fi

        else
            # Fallback to simple allow-only verification (legacy behavior)

            # Determine expected result based on filename
            if [[ "$filename" == ok*.json ]]; then
                expected="true"
            elif [[ "$filename" == nok*.json ]] || [[ "$filename" == nok-*.json ]]; then
                expected="false"
            else
                echo "⚠️  SKIPPED (unknown prefix, use 'ok' or 'nok')"
                ((total_tests--))
                continue
            fi

            # Merge FHIR base URL into input JSON (create context if it doesn't exist)
            temp_input=$(mktemp)
            jq --arg fhir_url "http://localhost:7623/fhir" \
                '.context = (.context // {}) | .context.fhir_base_url = $fhir_url' "$input_file" > "$temp_input"

            # Run OPA evaluation
            result=$(opa eval -i "$temp_input" -d "common.rego" -d "$policy_dir/policy.rego" "data.$package_name.allow" --format raw 2>&1 || echo "ERROR")

            # Clean up temp file
            rm -f "$temp_input"

            # Check result
            if [ "$result" == "ERROR" ] || [ -z "$result" ]; then
                echo "❌ ERROR (evaluation failed or timed out)"
                ((total_errors++))
                ((total_failed++))
            elif [ "$result" == "$expected" ]; then
                echo "✅ PASS (allow = $result)"
                ((total_passed++))
            else
                echo "❌ FAIL (expected: $expected, got: $result)"
                ((total_failed++))
            fi
        fi
    done

    echo ""
done

# Print summary
echo "==========================================="
echo "Test Summary"
echo "==========================================="
echo "Total tests:  $total_tests"
echo "✅ Passed:     $total_passed"
echo "❌ Failed:     $total_failed"
if [ $total_errors -gt 0 ]; then
    echo "⚠️  Errors:     $total_errors"
fi
echo ""

# Calculate pass rate
if [ $total_tests -gt 0 ]; then
    pass_rate=$((total_passed * 100 / total_tests))
    echo "Pass rate: $pass_rate%"
    echo ""
fi

# Exit with appropriate code
if [ $total_failed -eq 0 ] && [ $total_errors -eq 0 ]; then
    echo "🎉 All tests passed!"
    exit 0
else
    echo "❌ Some tests failed or had errors!"
    exit 1
fi

