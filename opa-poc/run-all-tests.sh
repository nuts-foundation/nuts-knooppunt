#!/bin/bash

# Script to run all OPA policy tests in subdirectories
# Run from the opa-poc directory

set -e

echo "==========================================="
echo "OPA Policy Test Runner"
echo "==========================================="
echo ""

# Check if opa is available
if ! command -v opa &> /dev/null; then
    echo "❌ Error: opa CLI not found!"
    echo "Please install it: brew install opa"
    exit 1
fi

echo "Using OPA version:"
opa version | head -1
echo ""

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

    # Find all JSON files in the directory
    json_files=$(find "$policy_dir" -maxdepth 1 -name "*.json" -type f | sort)

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

        # Run OPA evaluation
        result=$(opa eval -i "$input_file" -d "$policy_dir/policy.rego" "data.$package_name.allow" --format raw 2>&1 || echo "ERROR")

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

