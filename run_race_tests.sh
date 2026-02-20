#!/bin/bash

# Race Detection Test Suite for DRB Node
# This script runs comprehensive race condition tests with Go's race detector

set -e

echo "🔍 Starting Race Condition Detection Tests for DRB Node"
echo "========================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to run tests with race detector
run_race_test() {
    local test_path="$1"
    local test_name="$2"
    local timeout="${3:-5m}"
    
    echo -e "${BLUE}Running race tests for ${test_name}...${NC}"
    
    if go test -race -timeout="$timeout" -v "$test_path" 2>&1 | tee "race_test_${test_name}.log"; then
        echo -e "${GREEN}✅ PASSED: ${test_name}${NC}"
        return 0
    else
        echo -e "${RED}❌ FAILED: ${test_name} - Race conditions detected!${NC}"
        return 1
    fi
}

# Function to run stress tests
run_stress_test() {
    local test_path="$1"
    local test_name="$2"
    local count="${3:-100}"
    
    echo -e "${BLUE}Running stress test for ${test_name} (${count} iterations)...${NC}"
    
    for i in $(seq 1 "$count"); do
        if ! go test -race -count=1 "$test_path" > /dev/null 2>&1; then
            echo -e "${RED}❌ STRESS TEST FAILED: ${test_name} at iteration ${i}${NC}"
            return 1
        fi
        
        # Progress indicator
        if [ $((i % 10)) -eq 0 ]; then
            echo -e "${YELLOW}  Progress: ${i}/${count}${NC}"
        fi
    done
    
    echo -e "${GREEN}✅ STRESS TEST PASSED: ${test_name} (${count} iterations)${NC}"
    return 0
}

# Clean up previous logs
rm -f race_test_*.log goroutine_profile_*.txt

echo -e "${YELLOW}🧹 Cleaning up previous test artifacts...${NC}"

# Set up environment for testing
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=testuser
export POSTGRES_PASSWORD=testpass
export POSTGRES_DB=testdb
export NODE_TYPE=leader
export LEADER_PRIVATE_KEY=0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12
export CONTRACT_ADDRESS=0x1234567890123456789012345678901234567890
export ETH_RPC_URLS=http://localhost:8545

echo -e "${YELLOW}📋 Test Environment Setup Complete${NC}"

# Track overall results
FAILED_TESTS=()
PASSED_TESTS=()

echo
echo "🧪 PHASE 1: Core Race Detection Tests"
echo "====================================="

# Test leader node race conditions
if run_race_test "./nodes/leader" "leader_node" "10m"; then
    PASSED_TESTS+=("leader_node")
else
    FAILED_TESTS+=("leader_node")
fi

# Test regular node race conditions
if run_race_test "./nodes/regular" "regular_node" "10m"; then
    PASSED_TESTS+=("regular_node")
else
    FAILED_TESTS+=("regular_node")
fi

# Test utils race conditions
if run_race_test "./utils" "utils" "5m"; then
    PASSED_TESTS+=("utils")
else
    FAILED_TESTS+=("utils")
fi

# Test eth package race conditions
if run_race_test "./eth" "eth_service" "5m"; then
    PASSED_TESTS+=("eth_service")
else
    FAILED_TESTS+=("eth_service")
fi

# Test database race conditions
if run_race_test "./database" "database" "5m"; then
    PASSED_TESTS+=("database")
else
    FAILED_TESTS+=("database")
fi

echo
echo "🔥 PHASE 2: Concurrency Stress Tests"
echo "===================================="

# Test specific concurrency scenarios
if run_race_test "./nodes/leader" "leader_concurrency" "15m"; then
    PASSED_TESTS+=("leader_concurrency")
else
    FAILED_TESTS+=("leader_concurrency")
fi

# Test goroutine leak detection
if run_race_test "./testing" "goroutine_leaks" "10m"; then
    PASSED_TESTS+=("goroutine_leaks")
else
    FAILED_TESTS+=("goroutine_leaks")
fi

echo
echo "⚡ PHASE 3: High-Stress Repeated Tests"
echo "======================================"

# Run critical components under stress
echo -e "${BLUE}Testing critical paths under stress...${NC}"

# Stress test leader node operations
if run_stress_test "./nodes/leader" "leader_stress" 50; then
    PASSED_TESTS+=("leader_stress")
else
    FAILED_TESTS+=("leader_stress")
fi

# Stress test commit-reveal protocol
if run_stress_test "./commit-reveal2" "commit_reveal_stress" 30; then
    PASSED_TESTS+=("commit_reveal_stress")
else
    FAILED_TESTS+=("commit_reveal_stress")
fi

echo
echo "🔍 PHASE 4: Memory and Goroutine Analysis"
echo "========================================="

# Check for goroutine leaks in long-running tests
echo -e "${BLUE}Analyzing goroutine behavior...${NC}"

# Profile goroutines during test execution
go test -race -v "./testing" -run TestLongRunningOperations 2>&1 | \
    grep -E "(goroutine|race|leak)" > goroutine_analysis.log || true

# Run integration tests with race detector
echo -e "${BLUE}Running integration tests with race detection...${NC}"
if run_race_test "./integration_test" "integration" "20m"; then
    PASSED_TESTS+=("integration")
else
    FAILED_TESTS+=("integration")
fi

echo
echo "📊 RESULTS SUMMARY"
echo "=================="

echo -e "${GREEN}PASSED TESTS (${#PASSED_TESTS[@]}):${NC}"
for test in "${PASSED_TESTS[@]}"; do
    echo -e "  ✅ $test"
done

if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
    echo -e "${RED}FAILED TESTS (${#FAILED_TESTS[@]}):${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  ❌ $test"
    done
    
    echo
    echo -e "${RED}🚨 RACE CONDITIONS DETECTED!${NC}"
    echo -e "${YELLOW}Check the following log files for details:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        if [ -f "race_test_${test}.log" ]; then
            echo -e "  📄 race_test_${test}.log"
        fi
    done
    
    echo
    echo -e "${YELLOW}📋 Recommended Actions:${NC}"
    echo "1. Review race condition logs for specific issues"
    echo "2. Check mutex ordering and atomic operations"
    echo "3. Verify goroutine lifecycle management"
    echo "4. Test timer and channel operations"
    
    exit 1
else
    echo -e "${GREEN}🎉 ALL RACE DETECTION TESTS PASSED!${NC}"
    echo "The DRB node implementation appears to be free from race conditions."
fi

echo
echo -e "${BLUE}📈 Performance Notes:${NC}"
echo "- Run these tests regularly in CI/CD pipeline"
echo "- Monitor for performance degradation over time"
echo "- Consider running stress tests with higher iteration counts in staging"

# Cleanup
echo -e "${YELLOW}🧹 Cleaning up temporary files...${NC}"
rm -f race_test_*.log goroutine_*.txt

echo -e "${GREEN}✅ Race Detection Test Suite Complete${NC}"