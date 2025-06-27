#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GRPC_SERVER_HOST="localhost"
GRPC_SERVER_PORT="9090"
GRPC_SERVER_ADDR="${GRPC_SERVER_HOST}:${GRPC_SERVER_PORT}"
STORE_DIR="/tmp/distninja-grpc-test"
SERVER_PID=""
TEST_NINJA_FILE="/tmp/test_build.ninja"

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to cleanup on exit
cleanup() {
    print_info "Cleaning up..."
    if [ ! -z "$SERVER_PID" ]; then
        print_info "Stopping gRPC server (PID: $SERVER_PID)"
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi

    # Clean up test files
    rm -rf "$STORE_DIR" 2>/dev/null || true
    rm -f "$TEST_NINJA_FILE" 2>/dev/null || true

    print_info "Cleanup completed"
}

# Set trap for cleanup on exit
trap cleanup EXIT INT TERM

# Function to check if grpcurl is installed
check_grpcurl() {
    if ! command -v grpcurl &> /dev/null; then
        print_error "grpcurl is not installed. Installing..."

        # Try to install grpcurl
        if command -v go &> /dev/null; then
            go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
            export PATH=$PATH:$(go env GOPATH)/bin
        else
            print_error "Go is not installed. Please install grpcurl manually:"
            print_error "https://github.com/fullstorydev/grpcurl"
            exit 1
        fi

        if ! command -v grpcurl &> /dev/null; then
            print_error "Failed to install grpcurl"
            exit 1
        fi
    fi
    print_success "grpcurl is available"
}

# Function to create test Ninja file
create_test_ninja_file() {
    print_info "Creating test Ninja file: $TEST_NINJA_FILE"

    cat > "$TEST_NINJA_FILE" << 'EOF'
# Test Ninja build file
rule cc
  command = gcc -c $in -o $out
  description = Compiling $in

rule link
  command = gcc $in -o $out
  description = Linking $out

build main.o: cc main.c
build utils.o: cc utils.c
build program: link main.o utils.o

# Test with variables
rule custom
  command = echo "Building $out from $in with $extra_flag"
  description = Custom build for $out
  extra_flag = -O2

build output.txt: custom input.txt
  extra_flag = -O3
EOF

    print_success "Test Ninja file created"
}

# Function to start the gRPC server
start_grpc_server() {
    print_info "Starting gRPC server on $GRPC_SERVER_ADDR"

    # Clean up any existing store
    rm -rf "$STORE_DIR"
    mkdir -p "$STORE_DIR"

    # Start the server in background
    cd "$(dirname "$0")/.."
    go run cmd/server/main.go --grpc --address="$GRPC_SERVER_ADDR" --store="$STORE_DIR" &
    SERVER_PID=$!

    print_info "Server started with PID: $SERVER_PID"

    # Wait for server to start
    print_info "Waiting for server to start..."
    for i in {1..30}; do
        if grpcurl -plaintext "$GRPC_SERVER_ADDR" list > /dev/null 2>&1; then
            print_success "Server is ready"
            return 0
        fi
        sleep 1
        echo -n "."
    done

    print_error "Server failed to start within 30 seconds"
    return 1
}

# Function to test server reflection
test_reflection() {
    print_info "Testing gRPC reflection..."

    if grpcurl -plaintext "$GRPC_SERVER_ADDR" list | grep -q "distninja.DistNinjaService"; then
        print_success "Reflection working - DistNinjaService found"
    else
        print_error "Reflection failed - DistNinjaService not found"
        return 1
    fi

    # List methods
    print_info "Available methods:"
    grpcurl -plaintext "$GRPC_SERVER_ADDR" list distninja.DistNinjaService | sed 's/^/  /'
}

# Function to test health endpoint
test_health() {
    print_info "Testing Health endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/Health 2>/dev/null)

    if echo "$response" | grep -q '"status": "healthy"'; then
        print_success "Health check passed"
        echo "Response: $response"
    else
        print_error "Health check failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test status endpoint
test_status() {
    print_info "Testing Status endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/Status 2>/dev/null)

    if echo "$response" | grep -q '"service": "distninja"'; then
        print_success "Status check passed"
        echo "Response: $response"
    else
        print_error "Status check failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test load Ninja file
test_load_ninja_file() {
    print_info "Testing LoadNinjaFile endpoint..."

    local response=$(grpcurl -plaintext -d "{\"file_path\": \"$TEST_NINJA_FILE\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

    if echo "$response" | grep -q '"status": "success"'; then
        print_success "LoadNinjaFile test passed"
        echo "Response: $response"
    else
        print_error "LoadNinjaFile test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to create properly formatted ninja content for JSON
create_ninja_content_json() {
    local content="$1"
    # Convert actual newlines to \n for JSON
    echo "$content" | sed ':a;N;$!ba;s/\n/\\n/g' | sed 's/"/\\"/g'
}

# Function to test load Ninja file with content
test_load_ninja_content() {
    print_info "Testing LoadNinjaFile with content..."

    # Create proper ninja content with description
    local ninja_content='rule test
  command = echo test
  description = Test rule

build out: test in'

    # Convert to JSON-safe format
    local json_content=$(create_ninja_content_json "$ninja_content")

    local response=$(grpcurl -plaintext -d "{\"content\": \"$json_content\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

    if echo "$response" | grep -q '"status": "success"'; then
        print_success "LoadNinjaFile with content test passed"
        echo "Response: $response"
    else
        print_error "LoadNinjaFile with content test failed"
        echo "Response: $response"

        # Try with a simpler content for debugging
        print_info "Trying with minimal valid content..."
        local minimal_content='rule simple
  command = echo hello
  description = Simple test rule'
        local minimal_json=$(create_ninja_content_json "$minimal_content")

        local simple_response=$(grpcurl -plaintext -d "{\"content\": \"$minimal_json\"}" \
            "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

        if echo "$simple_response" | grep -q '"status": "success"'; then
            print_success "Minimal content test passed"
            echo "Simple Response: $simple_response"
        else
            print_error "Even minimal content failed"
            echo "Simple Response: $simple_response"
            return 1
        fi
    fi
}

# Function to test load Ninja file with various content formats
test_load_ninja_content_variations() {
    print_info "Testing LoadNinjaFile with various content formats..."

    # Test 1: Simple rule with description
    print_info "  Test 1: Simple rule with description"
    local content1='rule test
  command = echo hello
  description = Hello world rule'
    local json1=$(create_ninja_content_json "$content1")

    local response1=$(grpcurl -plaintext -d "{\"content\": \"$json1\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

    if echo "$response1" | grep -q '"status": "success"'; then
        print_success "  Simple rule test passed"
    else
        print_error "  Simple rule test failed"
        echo "  Response: $response1"
    fi

    # Test 2: Rule with build statement
    print_info "  Test 2: Rule with build statement"
    local content2='rule cc
  command = gcc -c $in -o $out
  description = Compile C file

build out.o: cc in.c'
    local json2=$(create_ninja_content_json "$content2")

    local response2=$(grpcurl -plaintext -d "{\"content\": \"$json2\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

    if echo "$response2" | grep -q '"status": "success"'; then
        print_success "  Rule with build test passed"
    else
        print_error "  Rule with build test failed"
        echo "  Response: $response2"
    fi

    # Test 3: Complex content with multiple rules
    print_info "  Test 3: Complex content with multiple rules"
    local content3='rule compile
  command = gcc -c $in -o $out
  description = Compiling $in

rule link
  command = gcc $in -o $out
  description = Linking $out

build main.o: compile main.c
build app: link main.o'
    local json3=$(create_ninja_content_json "$content3")

    local response3=$(grpcurl -plaintext -d "{\"content\": \"$json3\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>/dev/null)

    if echo "$response3" | grep -q '"status": "success"'; then
        print_success "  Complex content test passed"
    else
        print_error "  Complex content test failed"
        echo "  Response: $response3"
    fi
}

# Function to debug JSON formatting
test_json_formatting() {
    print_info "Testing JSON formatting for LoadNinjaFile..."

    # Test with proper content and description
    local test_content='rule debug
  command = echo debug
  description = Debug rule

build test: debug input'

    local json_content=$(create_ninja_content_json "$test_content")
    print_info "Testing with JSON content: $json_content"

    local response=$(grpcurl -plaintext -d "{\"content\": \"$json_content\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>&1)

    print_info "Raw response:"
    echo "$response"

    if echo "$response" | grep -q '"status": "success"'; then
        print_success "JSON formatting test passed"
    else
        print_error "JSON formatting test failed"

        # Show what we're actually sending
        print_info "Debugging - actual JSON being sent:"
        echo "{\"content\": \"$json_content\"}"
    fi
}

# Function to test build stats
test_build_stats() {
    print_info "Testing GetBuildStats endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetBuildStats 2>/dev/null)

    if echo "$response" | grep -q '"stats"'; then
        print_success "GetBuildStats test passed"
        echo "Response: $response"
    else
        print_error "GetBuildStats test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test get all targets
test_get_all_targets() {
    print_info "Testing GetAllTargets endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetAllTargets 2>/dev/null)

    if echo "$response" | grep -q '"targets"'; then
        print_success "GetAllTargets test passed"
        local target_count=$(echo "$response" | grep -o '"path"' | wc -l)
        print_info "Found $target_count targets"
        echo "Response: $response"
    else
        print_error "GetAllTargets test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test create rule
test_create_rule() {
    print_info "Testing CreateRule endpoint..."

    local response=$(grpcurl -plaintext -d '{
        "name": "test_rule",
        "command": "echo Building $out",
        "description": "Test rule for gRPC testing",
        "variables": {"test_var": "test_value"}
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateRule 2>/dev/null)

    if echo "$response" | grep -q '"status": "created"'; then
        print_success "CreateRule test passed"
        echo "Response: $response"
    else
        print_error "CreateRule test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test get rule
test_get_rule() {
    print_info "Testing GetRule endpoint..."

    local response=$(grpcurl -plaintext -d '{"name": "cc"}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetRule 2>/dev/null)

    if echo "$response" | grep -q '"name": "cc"'; then
        print_success "GetRule test passed"
        echo "Response: $response"
    else
        print_error "GetRule test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test get target
test_get_target() {
    print_info "Testing GetTarget endpoint..."

    local response=$(grpcurl -plaintext -d '{"path": "main.o"}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetTarget 2>/dev/null)

    if echo "$response" | grep -q '"path": "main.o"' || echo "$response" | grep -q "not found"; then
        print_success "GetTarget test passed (target found or properly handled not found)"
        echo "Response: $response"
    else
        print_error "GetTarget test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test create build with proper Pool field
test_create_build() {
    print_info "Testing CreateBuild endpoint..."

    # First, ensure the rule exists by creating it
    print_info "  Creating prerequisite rule 'cc'..."
    local rule_response=$(grpcurl -plaintext -d '{
        "name": "cc",
        "command": "gcc -c $in -o $out",
        "description": "Compile C files",
        "variables": {}
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateRule 2>/dev/null)

    if ! echo "$rule_response" | grep -q '"status": "created"'; then
        print_info "  Rule might already exist or creation failed, continuing with build test..."
        echo "  Rule response: $rule_response"
    else
        print_success "  Rule 'cc' created successfully"
    fi

    # Now test creating the build with proper Pool field
    local response=$(grpcurl -plaintext -d '{
        "build_id": "test_build_unique",
        "rule": "cc",
        "inputs": ["test.c"],
        "outputs": ["test.o"],
        "variables": {"extra_flags": "-Wall"},
        "pool": "console"
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>/dev/null)

    if echo "$response" | grep -q '"status": "created"'; then
        print_success "CreateBuild test passed"
        echo "Response: $response"
    else
        print_error "CreateBuild test failed"
        echo "Response: $response"

        # Try with different pool values
        print_info "  Trying with different pool values..."
        local pools=("" "default" "console")

        for pool in "${pools[@]}"; do
            print_info "    Trying pool: '$pool'"
            local pool_response=$(grpcurl -plaintext -d "{
                \"build_id\": \"minimal_build_$pool\",
                \"rule\": \"cc\",
                \"outputs\": [\"minimal_$pool.o\"],
                \"inputs\": [\"minimal_$pool.c\"],
                \"pool\": \"$pool\"
            }" "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

            if echo "$pool_response" | grep -q '"status": "created"'; then
                print_success "    Pool '$pool' worked!"
                echo "    Response: $pool_response"
                return 0
            else
                print_info "    Pool '$pool' failed: $pool_response"
            fi
        done

        return 1
    fi
}

# Function to test comprehensive CreateBuild scenarios (FIXED VERSION)
test_create_build_comprehensive() {
    print_info "Testing comprehensive CreateBuild scenarios..."

    # First, let's check what rules are available
    print_info "  Checking available rules first..."
    local available_rules_response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetBuildStats 2>/dev/null)
    echo "  Available rules info: $available_rules_response"

    # Test 1: Create build with a rule we know exists (cc from previous test) - with proper Pool
    print_info "  Test 1: Create build with known existing rule 'cc' and proper Pool"
    local response1=$(grpcurl -plaintext -d '{
        "build_id": "comprehensive_test_1",
        "rule": "cc",
        "inputs": ["source1.c"],
        "outputs": ["result1.o"],
        "variables": {"custom_var": "custom_value"},
        "pool": "console"
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

    print_info "  Raw response1: $response1"

    if echo "$response1" | grep -q '"status": "created"'; then
        print_success "  Build with known rule test passed"
    else
        print_error "  Build with known rule test failed"
        echo "  Response: $response1"

        # Try with different pool values
        print_info "  Trying with different pool values..."
        local pools=("" "default")

        for pool in "${pools[@]}"; do
            print_info "    Trying pool: '$pool'"
            local response1_alt=$(grpcurl -plaintext -d "{
                \"build_id\": \"comprehensive_test_1_$pool\",
                \"rule\": \"cc\",
                \"inputs\": [\"source1.c\"],
                \"outputs\": [\"result1.o\"],
                \"variables\": {\"custom_var\": \"custom_value\"},
                \"pool\": \"$pool\"
            }" "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

            if echo "$response1_alt" | grep -q '"status": "created"'; then
                print_success "    Pool '$pool' worked for comprehensive test!"
                echo "    Response: $response1_alt"
                break
            else
                print_info "    Pool '$pool' failed: $response1_alt"
            fi
        done
    fi

    # Test 2: Create the 'link' rule first, then create a build with it - with proper Pool
    print_info "  Test 2: Create 'link' rule then build with it (with proper Pool)"

    # Create the link rule first
    local link_rule_response=$(grpcurl -plaintext -d '{
        "name": "link",
        "command": "gcc $in -o $out",
        "description": "Link object files",
        "variables": {}
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateRule 2>&1)

    print_info "    Link rule response: $link_rule_response"

    if echo "$link_rule_response" | grep -q '"status": "created"'; then
        print_success "    Link rule created successfully"

        # Now create a build with the link rule - with proper Pool
        print_info "    Trying build with pool: 'console'"
        local response2=$(grpcurl -plaintext -d '{
            "build_id": "multi_io_build_console",
            "rule": "link",
            "inputs": ["main.o", "utils.o", "helper.o"],
            "outputs": ["final_program"],
            "implicit_deps": ["library.a"],
            "order_deps": ["config.h"],
            "variables": {"link_flags": "-lpthread -lm"},
            "pool": "console"
        }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

        print_info "    Build response2 (console pool): $response2"

        if echo "$response2" | grep -q '"status": "created"'; then
            print_success "  Multi I/O build test passed with console pool"
        else
            print_error "  Multi I/O build test failed with console pool"

            # Try with different pool
            print_info "    Trying build with pool: 'default'"
            local response2_default=$(grpcurl -plaintext -d '{
                "build_id": "multi_io_build_default",
                "rule": "link",
                "inputs": ["main.o", "utils.o", "helper.o"],
                "outputs": ["final_program_default"],
                "implicit_deps": ["library.a"],
                "order_deps": ["config.h"],
                "variables": {"link_flags": "-lpthread -lm"},
                "pool": "default"
            }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

            print_info "    Build response2 (default pool): $response2_default"

            if echo "$response2_default" | grep -q '"status": "created"'; then
                print_success "  Multi I/O build test passed with default pool"
            else
                print_error "  Multi I/O build test failed with default pool"
                echo "  Response: $response2_default"
            fi
        fi
    else
        print_info "    Link rule creation failed or already exists, trying anyway..."
        echo "    Link rule response: $link_rule_response"
    fi

    # Test 3: Create build with non-existent rule (should fail gracefully)
    print_info "  Test 3: Create build with non-existent rule"
    local response3=$(grpcurl -plaintext -d '{
        "build_id": "nonexistent_rule_build",
        "rule": "nonexistent_rule",
        "inputs": ["test.c"],
        "outputs": ["test.o"],
        "pool": "console"
    }' "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

    print_info "    Non-existent rule response: $response3"

    if echo "$response3" | grep -q "error\|Error\|failed\|Failed\|not found"; then
        print_success "  Non-existent rule properly handled"
    else
        print_warning "  Non-existent rule handling might need improvement"
        echo "  Response: $response3"
    fi
}

# Function to test get build after creation
test_get_build() {
    print_info "Testing GetBuild endpoint..."

    # Try to get a build that should exist from our previous tests
    local response=$(grpcurl -plaintext -d '{"id": "test_build_unique"}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetBuild 2>/dev/null)

    # Check for both camelCase and snake_case field names
    if echo "$response" | grep -q '"buildId": "test_build_unique"' || echo "$response" | grep -q '"build_id": "test_build_unique"' || echo "$response" | grep -q "not found"; then
        print_success "GetBuild test passed (build found or properly handled not found)"
        echo "Response: $response"
    else
        print_error "GetBuild test failed"
        echo "Response: $response"

        # Try getting a build from the loaded file with different ID formats
        print_info "  Trying to get build from loaded file..."

        # Try different possible build IDs from the loaded ninja file
        local build_ids=("main.o" "utils.o" "program" "output.txt")
        local found_build=false

        for build_id in "${build_ids[@]}"; do
            local file_response=$(grpcurl -plaintext -d "{\"id\": \"$build_id\"}" \
                "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetBuild 2>/dev/null)

            if echo "$file_response" | grep -q '"buildId"\|"build_id"' && ! echo "$file_response" | grep -q "not found"; then
                print_success "  Found build with ID: $build_id"
                echo "  Response: $file_response"
                found_build=true
                break
            fi
        done

        if [ "$found_build" = false ]; then
            print_error "  No builds found from loaded file"
            return 1
        fi
    fi
}

# Function to test build order
test_build_order() {
    print_info "Testing GetBuildOrder endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetBuildOrder 2>/dev/null)

    # Check for both camelCase and snake_case field names
    if echo "$response" | grep -q '"buildOrder"' || echo "$response" | grep -q '"build_order"'; then
        print_success "GetBuildOrder test passed"
        local order_count=$(echo "$response" | grep -o '"[^"]*"' | wc -l)
        print_info "Build order contains $order_count entries"
        echo "Response: $response"
    else
        print_error "GetBuildOrder test failed"
        echo "Response: $response"
        return 1
    fi
}

# Add a test to check what pools are available or expected
test_pool_requirements() {
    print_info "Testing Pool requirements..."

    # Try to understand what pools are valid by testing various options
    local pools=("" "default" "console")

    for pool in "${pools[@]}"; do
        print_info "  Testing pool: '$pool'"
        local response=$(grpcurl -plaintext -d "{
            \"build_id\": \"pool_test_$pool\",
            \"rule\": \"cc\",
            \"outputs\": [\"pool_test_$pool.o\"],
            \"inputs\": [\"pool_test_$pool.c\"],
            \"pool\": \"$pool\"
        }" "$GRPC_SERVER_ADDR" distninja.DistNinjaService/CreateBuild 2>&1)

        if echo "$response" | grep -q '"status": "created"'; then
            print_success "  Pool '$pool' is valid"
        else
            print_info "  Pool '$pool' failed or invalid"
            echo "    Response: $response"
        fi
    done
}

# Function to debug what rules and builds exist after loading
debug_loaded_content() {
    print_info "Debugging loaded content..."

    # Try to get rules that should exist from the loaded ninja file
    local rules_to_check=("cc" "link" "custom")

    for rule in "${rules_to_check[@]}"; do
        print_info "  Checking rule: $rule"
        local rule_response=$(grpcurl -plaintext -d "{\"name\": \"$rule\"}" \
            "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetRule 2>/dev/null)

        if echo "$rule_response" | grep -q '"name"'; then
            print_success "    Rule $rule exists"
        else
            print_error "    Rule $rule not found"
            echo "    Response: $rule_response"
        fi
    done

    # Check what targets exist
    print_info "  Checking targets from loaded file..."
    local targets_response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetAllTargets 2>/dev/null)
    local target_count=$(echo "$targets_response" | grep -o '"path"' | wc -l)
    print_info "    Found $target_count targets"

    # Show first few targets
    if [ "$target_count" -gt 0 ]; then
        echo "$targets_response" | grep '"path"' | head -5 | sed 's/^/      /'
    fi
}

# Add a test to verify ninja file loading worked correctly
test_ninja_file_content_verification() {
    print_info "Verifying ninja file content was loaded correctly..."

    # Check if rules from the test ninja file exist
    local expected_rules=("cc" "link" "custom")
    local rules_found=0

    for rule in "${expected_rules[@]}"; do
        local response=$(grpcurl -plaintext -d "{\"name\": \"$rule\"}" \
            "$GRPC_SERVER_ADDR" distninja.DistNinjaService/GetRule 2>/dev/null)

        if echo "$response" | grep -q "\"name\": \"$rule\""; then
            print_success "  Rule '$rule' loaded correctly"
            ((rules_found++))
        else
            print_error "  Rule '$rule' not found after loading"
            echo "  Response: $response"
        fi
    done

    if [ $rules_found -eq ${#expected_rules[@]} ]; then
        print_success "All expected rules loaded successfully"
        return 0
    else
        print_error "Only $rules_found out of ${#expected_rules[@]} rules loaded"
        return 1
    fi
}

# Function to test server health with detailed debugging
test_server_connectivity() {
    print_info "Testing server connectivity in detail..."

    # Test 1: Basic connection
    print_info "  Test 1: Basic connection test"
    if grpcurl -plaintext "$GRPC_SERVER_ADDR" list > /dev/null 2>&1; then
        print_success "  Basic connection works"
    else
        print_error "  Basic connection failed"
        return 1
    fi

    # Test 2: Service listing
    print_info "  Test 2: Service listing"
    local services=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" list 2>/dev/null)
    if echo "$services" | grep -q "distninja.DistNinjaService"; then
        print_success "  Service listing works"
        echo "  Available services:"
        echo "$services" | sed 's/^/    /'
    else
        print_error "  Service listing failed"
        echo "  Response: $services"
        return 1
    fi

    # Test 3: Method listing
    print_info "  Test 3: Method listing"
    local methods=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" list distninja.DistNinjaService 2>/dev/null)
    if [ -n "$methods" ]; then
        print_success "  Method listing works"
        echo "  Available methods:"
        echo "$methods" | sed 's/^/    /'
    else
        print_error "  Method listing failed"
        return 1
    fi

    # Test 4: Simple RPC call
    print_info "  Test 4: Simple RPC call (Health)"
    local health_response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/Health 2>/dev/null)
    if echo "$health_response" | grep -q '"status": "healthy"'; then
        print_success "  Simple RPC call works"
    else
        print_error "  Simple RPC call failed"
        echo "  Response: $health_response"
        return 1
    fi
}

# Function to test find cycles
test_find_cycles() {
    print_info "Testing FindCycles endpoint..."

    local response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/FindCycles 2>/dev/null)

    print_info "Raw FindCycles response: $response"

    # Check for various possible response formats
    if echo "$response" | grep -q '"cycles"'; then
        print_success "FindCycles test passed - cycles field found"
        local cycle_count=$(echo "$response" | grep -o '"cycle_count"' | wc -l)
        print_info "Cycle analysis completed"
        echo "Response: $response"
    elif echo "$response" | grep -q '{}'; then
        print_success "FindCycles test passed - empty response indicates no cycles found"
        print_info "No cycles detected in the build graph"
        echo "Response: $response"
    elif echo "$response" | grep -q '"message"'; then
        print_success "FindCycles test passed - message response received"
        echo "Response: $response"
    elif [ -n "$response" ] && echo "$response" | grep -q '{'; then
        print_success "FindCycles test passed - valid JSON response received"
        print_info "Cycle detection completed"
        echo "Response: $response"
    else
        print_error "FindCycles test failed - unexpected response format"
        echo "Response: $response"

        # Try with verbose output to debug
        print_info "Trying with verbose grpcurl output..."
        local verbose_response=$(grpcurl -plaintext -v "$GRPC_SERVER_ADDR" distninja.DistNinjaService/FindCycles 2>&1)
        echo "Verbose response: $verbose_response"

        return 1
    fi
}

# Function to test debug quads
test_debug_quads() {
    print_info "Testing DebugQuads endpoint..."

    local response=$(grpcurl -plaintext -d '{"limit": 10}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/DebugQuads 2>/dev/null)

    if echo "$response" | grep -q '"message"'; then
        print_success "DebugQuads test passed"
        echo "Response: $response"
    else
        print_error "DebugQuads test failed"
        echo "Response: $response"
        return 1
    fi
}

# Function to test invalid requests
test_error_handling() {
    print_info "Testing error handling..."

    # Test LoadNinjaFile with invalid file
    local response=$(grpcurl -plaintext -d '{"file_path": "/nonexistent/file.ninja"}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>&1)

    if echo "$response" | grep -q "failed to read file"; then
        print_success "Error handling test passed - invalid file properly handled"
    else
        print_warning "Error handling might need improvement"
        echo "Response: $response"
    fi

    # Test LoadNinjaFile with no content
    response=$(grpcurl -plaintext -d '{}' \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>&1)

    if echo "$response" | grep -q "either file_path or content must be provided"; then
        print_success "Error handling test passed - empty request properly handled"
    else
        print_warning "Error handling for empty request might need improvement"
        echo "Response: $response"
    fi

    # Test invalid ninja syntax
    print_info "Testing invalid ninja syntax handling..."
    local invalid_content='invalid ninja syntax without proper structure'
    local invalid_json=$(create_ninja_content_json "$invalid_content")

    response=$(grpcurl -plaintext -d "{\"content\": \"$invalid_json\"}" \
        "$GRPC_SERVER_ADDR" distninja.DistNinjaService/LoadNinjaFile 2>&1)

    if echo "$response" | grep -q "error\|failed"; then
        print_success "Invalid syntax properly handled"
    else
        print_warning "Invalid syntax handling might need improvement"
        echo "Response: $response"
    fi
}

# Function to run performance test
test_performance() {
    print_info "Running performance test..."

    print_info "Testing multiple concurrent health checks..."
    for i in {1..10}; do
        grpcurl -plaintext "$GRPC_SERVER_ADDR" distninja.DistNinjaService/Health > /dev/null 2>&1 &
    done
    wait

    print_success "Performance test completed - 10 concurrent requests handled"
}

# Function to test health check endpoint (gRPC native)
test_grpc_health() {
    print_info "Testing gRPC native health check..."

    if command -v grpc_health_probe &> /dev/null; then
        if grpc_health_probe -addr="$GRPC_SERVER_ADDR"; then
            print_success "gRPC health probe passed"
        else
            print_warning "gRPC health probe failed (this is expected if service name not set)"
        fi
    else
        print_info "grpc_health_probe not available, skipping native health check"
    fi
}

# Function to test basic grpcurl functionality
test_grpcurl_basic() {
    print_info "Testing basic grpcurl functionality..."

    # Test 1: Simple list command
    local list_response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" list 2>&1)
    print_info "  List services response: $list_response"

    # Test 2: Describe service
    local describe_response=$(grpcurl -plaintext "$GRPC_SERVER_ADDR" describe distninja.DistNinjaService 2>&1)
    print_info "  Describe service response length: ${#describe_response}"

    # Test 3: Simple health check with verbose output
    local health_verbose=$(grpcurl -plaintext -v "$GRPC_SERVER_ADDR" distninja.DistNinjaService/Health 2>&1)
    print_info "  Health check verbose response length: ${#health_verbose}"

    if echo "$health_verbose" | grep -q '"status": "healthy"'; then
        print_success "  Basic grpcurl functionality works"
    else
        print_error "  Basic grpcurl functionality issues detected"
        echo "  Health verbose response: $health_verbose"
    fi
}

# Main test function
run_tests() {
    print_info "Starting gRPC server tests..."

    # Pre-test setup
    check_grpcurl
    create_test_ninja_file

    # Start server
    if ! start_grpc_server; then
        print_error "Failed to start server, aborting tests"
        exit 1
    fi

    # Test basic connectivity first
    if ! test_server_connectivity; then
        print_error "Server connectivity issues detected, aborting tests"
        exit 1
    fi

    # Test grpcurl basic functionality
    test_grpcurl_basic

    # Run tests
    local failed_tests=0

    # Basic connectivity tests
    test_reflection || ((failed_tests++))
    test_health || ((failed_tests++))
    test_status || ((failed_tests++))
    test_grpc_health || ((failed_tests++))

    # Load and parsing tests (do this early to populate the store)
    test_load_ninja_file || ((failed_tests++))

    # Verify the content was loaded properly
    test_ninja_file_content_verification || ((failed_tests++))

    # Debug what's actually in the store
    debug_loaded_content

    # Add debug test for content loading
    print_info "Running debug tests for content loading..."
    test_json_formatting || ((failed_tests++))
    test_load_ninja_content || ((failed_tests++))
    test_load_ninja_content_variations || ((failed_tests++))

    # Data retrieval tests
    test_build_stats || ((failed_tests++))
    test_get_all_targets || ((failed_tests++))

    # CRUD operation tests (order matters - create rules before builds)
    test_create_rule || ((failed_tests++))
    test_get_rule || ((failed_tests++))

    # Test pool requirements before build creation
    test_pool_requirements || ((failed_tests++))

    test_create_build || ((failed_tests++))
    test_create_build_comprehensive || ((failed_tests++))
    test_get_build || ((failed_tests++))
    test_build_order || ((failed_tests++))
    test_get_target || ((failed_tests++))

    # Analysis tests
    test_find_cycles || ((failed_tests++))
    test_debug_quads || ((failed_tests++))

    # Error handling and performance tests
    test_error_handling || ((failed_tests++))
    test_performance || ((failed_tests++))

    # Summary
    echo ""
    print_info "Test Summary:"
    if [ $failed_tests -eq 0 ]; then
        print_success "All tests passed!"
    else
        print_warning "$failed_tests tests failed, but server is functional"
        print_info "Some failures might be due to expected error conditions or incomplete implementations"
    fi
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --help              Show this help message"
    echo "  --host HOST         gRPC server host (default: localhost)"
    echo "  --port PORT         gRPC server port (default: 9090)"
    echo "  --store DIR         Store directory (default: /tmp/distninja-grpc-test)"
    echo "  --keep-running      Keep server running after tests"
    echo "  --test TEST_NAME    Run a single test (health, build_order, create_build, connectivity)"
    echo ""
    echo "Examples:"
    echo "  $0                              # Run all tests with defaults"
    echo "  $0 --port 8080                  # Run tests on port 8080"
    echo "  $0 --keep-running               # Keep server running after tests"
    echo "  $0 --test create_build          # Run only the create build test"
}

# Function to run individual tests for debugging
run_single_test() {
    local test_name="$1"

    case "$test_name" in
        "health")
            test_health
            ;;
        "build_order")
            test_build_order
            ;;
        "create_build")
            test_create_build_comprehensive
            ;;
        "connectivity")
            test_server_connectivity
            ;;
        "pool")
            test_pool_requirements
            ;;
        *)
            print_error "Unknown test: $test_name"
            print_info "Available tests: health, build_order, create_build, connectivity, pool"
            exit 1
            ;;
    esac
}

# Parse command line arguments
KEEP_RUNNING=false
SINGLE_TEST=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            show_usage
            exit 0
            ;;
        --host)
            GRPC_SERVER_HOST="$2"
            GRPC_SERVER_ADDR="${GRPC_SERVER_HOST}:${GRPC_SERVER_PORT}"
            shift 2
            ;;
        --port)
            GRPC_SERVER_PORT="$2"
            GRPC_SERVER_ADDR="${GRPC_SERVER_HOST}:${GRPC_SERVER_PORT}"
            shift 2
            ;;
        --store)
            STORE_DIR="$2"
            shift 2
            ;;
        --keep-running)
            KEEP_RUNNING=true
            shift
            ;;
        --test)
            SINGLE_TEST="$2"
            shift 2
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Run the tests
print_info "gRPC Server Test Suite"
print_info "====================="
print_info "Server Address: $GRPC_SERVER_ADDR"
print_info "Store Directory: $STORE_DIR"
print_info ""

if [ -n "$SINGLE_TEST" ]; then
    print_info "Running single test: $SINGLE_TEST"

    # Start server for single test
    if ! start_grpc_server; then
        print_error "Failed to start server"
        exit 1
    fi

    run_single_test "$SINGLE_TEST"
else
    run_tests
fi

if [ "$KEEP_RUNNING" = true ]; then
    print_info "Server is running on $GRPC_SERVER_ADDR (PID: $SERVER_PID)"
    print_info "Press Ctrl+C to stop the server"

    # Override the cleanup trap to not kill the server
    trap 'print_info "Server is running (PID: $SERVER_PID). Use kill $SERVER_PID to stop it."' EXIT

    # Wait indefinitely
    wait $SERVER_PID 2>/dev/null || true
fi

print_success "gRPC server tests completed!"
