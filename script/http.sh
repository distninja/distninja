#!/bin/bash

set -e

# Configuration
BASE_URL="http://localhost:9090"
API_BASE="$BASE_URL/api/v1"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper function to print test results
print_test() {
    echo -e "${YELLOW}Testing: $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Test function
test_endpoint() {
    local method=$1
    local endpoint=$2
    local data=$3
    local expected_status=$4
    local description=$5

    print_test "$description"

    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$endpoint")
    fi

    # Extract status code (last line) and body (everything else)
    status_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n -1)

    if [ "$status_code" = "$expected_status" ]; then
        print_success "Status: $status_code"
        echo "Response: $body" | jq . 2>/dev/null || echo "Response: $body"
    else
        print_error "Expected status $expected_status, got $status_code"
        echo "Response: $body"
    fi
    echo "---"
}

echo "Starting HTTP API tests for distninja..."
echo "Base URL: $BASE_URL"
echo "=================================="

# Health check
test_endpoint "GET" "$BASE_URL/health" "" "200" "Health check"

# Status check
test_endpoint "GET" "$API_BASE/status" "" "200" "Service status"

# Create a rule first (needed for builds)
rule_data='{
    "name": "test_compile",
    "command": "gcc -o $out $in",
    "description": "Compile C file",
    "variables": {
        "cflags": "-O2"
    }
}'
test_endpoint "POST" "$API_BASE/rules" "$rule_data" "201" "Create rule"

# Get the created rule
test_endpoint "GET" "$API_BASE/rules/test_compile" "" "200" "Get rule"

# Create a build
build_data='{
    "build_id": "build_001",
    "rule": "test_compile",
    "variables": {
        "cflags": "-O3"
    },
    "pool": "highmem_pool",
    "inputs": ["main.c", "utils.c"],
    "outputs": ["main.o"],
    "implicit_deps": ["config.h"],
    "order_deps": ["generated_headers"]
}'
test_endpoint "POST" "$API_BASE/builds" "$build_data" "201" "Create build"

# Get the created build
test_endpoint "GET" "$API_BASE/builds/build_001" "" "200" "Get build"

# Create another build for testing
build_data2='{
    "build_id": "build_002",
    "rule": "test_compile",
    "pool": "highmem_pool",
    "inputs": ["helper.c"],
    "outputs": ["helper.o"]
}'
test_endpoint "POST" "$API_BASE/builds" "$build_data2" "201" "Create second build"

# Get build statistics
test_endpoint "GET" "$API_BASE/builds/stats" "" "200" "Get build statistics"

# Get build order
test_endpoint "GET" "$API_BASE/builds/order" "" "200" "Get build order"

# Get targets by rule
test_endpoint "GET" "$API_BASE/rules/test_compile/targets" "" "200" "Get targets by rule"

# Get all targets
test_endpoint "GET" "$API_BASE/targets" "" "200" "Get all targets"

# Get specific target
test_endpoint "GET" "$API_BASE/targets/main.o" "" "200" "Get specific target"

# Get target dependencies
test_endpoint "GET" "$API_BASE/targets/main.o/dependencies" "" "200" "Get target dependencies"

# Get target reverse dependencies
test_endpoint "GET" "$API_BASE/targets/main.c/reverse_dependencies" "" "200" "Get target reverse dependencies"

# Update target status
status_data='{"status": "building"}'
test_endpoint "PUT" "$API_BASE/targets/main.o/status" "$status_data" "200" "Update target status"

# Find cycles
test_endpoint "GET" "$API_BASE/analysis/cycles" "" "200" "Find dependency cycles"

# Debug quads (limited)
test_endpoint "GET" "$API_BASE/debug/quads?limit=10" "" "200" "Debug quads (limited)"

# Test error cases
echo -e "${YELLOW}Testing error cases...${NC}"

# Invalid JSON
test_endpoint "POST" "$API_BASE/builds" '{"invalid": json}' "400" "Invalid JSON in build creation"

# Non-existent build
test_endpoint "GET" "$API_BASE/builds/nonexistent" "" "404" "Get non-existent build"

# Non-existent rule
test_endpoint "GET" "$API_BASE/rules/nonexistent" "" "404" "Get non-existent rule"

# Non-existent target
test_endpoint "GET" "$API_BASE/targets/nonexistent.o" "" "404" "Get non-existent target"

# Invalid target status update - missing status field
invalid_status1='{"invalid": "data"}'
test_endpoint "PUT" "$API_BASE/targets/main.o/status" "$invalid_status1" "400" "Invalid status update - missing status field"

# Invalid target status update - empty status
invalid_status3='{"status": ""}'
test_endpoint "PUT" "$API_BASE/targets/main.o/status" "$invalid_status3" "400" "Invalid status update - empty status"

# Update status for non-existent target
valid_status='{"status": "building"}'
test_endpoint "PUT" "$API_BASE/targets/nonexistent.o/status" "$valid_status" "404" "Update status for non-existent target"

# Test CORS preflight
echo -e "${YELLOW}Testing CORS preflight...${NC}"
cors_response=$(curl -s -w "\n%{http_code}" -X OPTIONS \
    -H "Origin: http://localhost:3000" \
    -H "Access-Control-Request-Method: POST" \
    -H "Access-Control-Request-Headers: Content-Type" \
    "$API_BASE/builds")

cors_status=$(echo "$cors_response" | tail -n1)
if [ "$cors_status" = "200" ]; then
    print_success "CORS preflight: $cors_status"
else
    print_error "CORS preflight failed: $cors_status"
fi

echo "=================================="
echo "Tests completed!"
echo ""
echo "Note: Make sure the distninja server is running on $BASE_URL"
echo "Example: go run main.go --http :9090 --store /tmp/ninja.db"
