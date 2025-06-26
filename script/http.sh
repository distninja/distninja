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

# Test Ninja file loading endpoint
echo -e "${YELLOW}Testing Ninja file loading...${NC}"

# Create temporary Ninja files for testing
temp_ninja_simple="/tmp/test_simple.ninja"
cat > "$temp_ninja_simple" << 'EOF'
rule cc
  command = gcc -c -o $out $in
  description = Compile C source

rule link
  command = gcc -o $out $in
  description = Link executable

build main.o: cc main.c | config.h
build helper.o: cc helper.c
build app: link main.o helper.o
EOF

temp_ninja_complex="/tmp/test_complex.ninja"
cat > "$temp_ninja_complex" << 'EOF'
pool highmem
  depth = 1

rule cc
  command = gcc $cflags -c -o $out $in
  description = Compile $out
  depfile = $out.d
  deps = gcc

rule ar
  command = ar rcs $out $in
  description = Archive $out

variable cflags = -O2 -Wall

build src/main.o: cc src/main.c | src/common.h
  cflags = -O3 -DDEBUG
  pool = highmem

build src/utils.o: cc src/utils.c

build lib/mylib.a: ar src/main.o src/utils.o

build bin/app: cc lib/mylib.a || generated_headers
EOF

# Test 1: Load Ninja file with file path
ninja_file_path_simple="{\"file_path\": \"$temp_ninja_simple\"}"
test_endpoint "POST" "$API_BASE/load" "$ninja_file_path_simple" "200" "Load simple Ninja file from file path"

# Test 2: Load complex Ninja file with file path
ninja_file_path_complex="{\"file_path\": \"$temp_ninja_complex\"}"
test_endpoint "POST" "$API_BASE/load" "$ninja_file_path_complex" "200" "Load complex Ninja file from file path"

# Test 3: Load Ninja file with content (properly escaped JSON)
ninja_content_json=$(jq -n --arg content "$(cat "$temp_ninja_simple")" '{"content": $content}')
test_endpoint "POST" "$API_BASE/load" "$ninja_content_json" "200" "Load Ninja file with content in JSON"

# Test 4: Load Ninja file with file path (create a temporary file first)
temp_ninja_file="/tmp/test_build.ninja"
cat > "$temp_ninja_file" << 'EOF'
# Test Ninja file
rule compile
  command = clang -c -o $out $in
  description = Compiling $in

rule link
  command = clang -o $out $in
  description = Linking $out

build obj/test.o: compile src/test.c
build bin/test: link obj/test.o
EOF

ninja_file_path="{\"file_path\": \"$temp_ninja_file\"}"
test_endpoint "POST" "$API_BASE/load" "$ninja_file_path" "200" "Load Ninja file from file path"

# Test 5: Verify loaded data by checking stats
test_endpoint "GET" "$API_BASE/builds/stats" "" "200" "Get build stats after loading"

# Test 6: Verify loaded rules
test_endpoint "GET" "$API_BASE/rules/cc" "" "200" "Get loaded 'cc' rule"
test_endpoint "GET" "$API_BASE/rules/link" "" "200" "Get loaded 'link' rule"
test_endpoint "GET" "$API_BASE/rules/compile" "" "200" "Get loaded 'compile' rule"

# Test 7: Verify loaded targets
test_endpoint "GET" "$API_BASE/targets/main.o" "" "200" "Get loaded 'main.o' target"
test_endpoint "GET" "$API_BASE/targets/src/main.o" "" "200" "Get loaded 'src/main.o' target"
test_endpoint "GET" "$API_BASE/targets/bin/app" "" "200" "Get loaded 'bin/app' target"

# Test 8: Check dependencies after loading
test_endpoint "GET" "$API_BASE/targets/src/main.o/dependencies" "" "200" "Get dependencies for loaded target"
test_endpoint "GET" "$API_BASE/targets/src/main.c/reverse_dependencies" "" "200" "Get reverse dependencies for loaded file"

# Test 9: Large Ninja file simulation (more realistic)
temp_ninja_large="/tmp/test_large.ninja"
cat > "$temp_ninja_large" << 'EOF'
# Generated by CMake

rule CXX_COMPILER__hello_Debug
  depfile = $DEP_FILE
  deps = gcc
  command = /usr/bin/c++ $DEFINES $INCLUDES $FLAGS -MD -MT $out -MF $DEP_FILE -o $out -c $in
  description = Building CXX object $out

rule CXX_EXECUTABLE_LINKER__hello_Debug
  command = /usr/bin/c++ $FLAGS $LINK_FLAGS $in -o $out $LINK_PATH $LINK_LIBRARIES
  description = Linking CXX executable $out

build CMakeFiles/hello.dir/main.cpp.o: CXX_COMPILER__hello_Debug /home/user/project/main.cpp || cmake_object_order_depends_target_hello
  DEP_FILE = CMakeFiles/hello.dir/main.cpp.o.d
  FLAGS = -g
  INCLUDES = -I/home/user/project/include
  OBJECT_DIR = CMakeFiles/hello.dir
  OBJECT_FILE_DIR = CMakeFiles/hello.dir

build CMakeFiles/hello.dir/utils.cpp.o: CXX_COMPILER__hello_Debug /home/user/project/utils.cpp || cmake_object_order_depends_target_hello
  DEP_FILE = CMakeFiles/hello.dir/utils.cpp.o.d
  FLAGS = -g
  INCLUDES = -I/home/user/project/include
  OBJECT_DIR = CMakeFiles/hello.dir
  OBJECT_FILE_DIR = CMakeFiles/hello.dir

build hello: CXX_EXECUTABLE_LINKER__hello_Debug CMakeFiles/hello.dir/main.cpp.o CMakeFiles/hello.dir/utils.cpp.o
  FLAGS = -g
  LINK_LIBRARIES = -lpthread
  OBJECT_DIR = CMakeFiles/hello.dir
  POST_BUILD = :
  PRE_LINK = :
  TARGET_FILE = hello
  TARGET_PDB = hello.dbg
EOF

large_ninja_path="{\"file_path\": \"$temp_ninja_large\"}"
test_endpoint "POST" "$API_BASE/load" "$large_ninja_path" "200" "Load large realistic Ninja file (CMake-style)"

# Test error cases for load endpoint
echo -e "${YELLOW}Testing load endpoint error cases...${NC}"

# Test 10: Invalid JSON
test_endpoint "POST" "$API_BASE/load" '{"invalid": json}' "400" "Load with invalid JSON"

# Test 11: Missing both file_path and content
empty_load_data='{"neither": "file_path_nor_content"}'
test_endpoint "POST" "$API_BASE/load" "$empty_load_data" "400" "Load with neither file_path nor content"

# Test 12: Non-existent file path
nonexistent_file_data='{"file_path": "/nonexistent/path/to/build.ninja"}'
test_endpoint "POST" "$API_BASE/load" "$nonexistent_file_data" "400" "Load with non-existent file path"

# Test 13: Empty content
empty_content_data='{"content": ""}'
test_endpoint "POST" "$API_BASE/load" "$empty_content_data" "200" "Load with empty content"

# Test 14: Very large content (performance test)
temp_ninja_perf="/tmp/test_performance.ninja"
cat > "$temp_ninja_perf" << 'EOF'
rule big_rule
  command = echo processing
  description = Big rule

EOF

for i in {1..100}; do
    echo "build output_${i}.o: big_rule input_${i}.c" >> "$temp_ninja_perf"
done

large_ninja_perf="{\"file_path\": \"$temp_ninja_perf\"}"
test_endpoint "POST" "$API_BASE/load" "$large_ninja_perf" "200" "Load with large content (100 build statements)"

# Test 15: Ninja file with line continuations
temp_ninja_continuation="/tmp/test_continuation.ninja"
cat > "$temp_ninja_continuation" << 'EOF'
rule compile_with_long_command
  command = gcc -std=c99 -Wall -Wextra -Werror $
    -O2 -DNDEBUG -I./include -I./external/include $
    -c -o $out $in
  description = Compile with long command line

build very/long/path/to/output/file.o: compile_with_long_command $
    very/long/path/to/source/file.c | $
    very/long/path/to/header1.h $
    very/long/path/to/header2.h
EOF

continuation_ninja_path="{\"file_path\": \"$temp_ninja_continuation\"}"
test_endpoint "POST" "$API_BASE/load" "$continuation_ninja_path" "200" "Load Ninja file with line continuations"

# Test 16: Check that multiple loads work (incremental loading)
temp_ninja_incremental="/tmp/test_incremental.ninja"
cat > "$temp_ninja_incremental" << 'EOF'
rule incremental_rule
  command = touch $out
  description = Incremental build

build incremental_output.txt: incremental_rule incremental_input.txt
EOF

incremental_ninja_path="{\"file_path\": \"$temp_ninja_incremental\"}"
test_endpoint "POST" "$API_BASE/load" "$incremental_ninja_path" "200" "Load additional Ninja content (incremental)"

# Final verification after all loads
test_endpoint "GET" "$API_BASE/builds/stats" "" "200" "Final build stats after all loads"

# Clean up temporary files
rm -f "$temp_ninja_simple" "$temp_ninja_complex" "$temp_ninja_file" "$temp_ninja_large" "$temp_ninja_perf" "$temp_ninja_continuation" "$temp_ninja_incremental"

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
echo "Example: go run main.go serve --http :9090 --store /tmp/ninja.db"
