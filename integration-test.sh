#!/bin/bash

# Integration Test Script
# Tests the full chain: OpenCode -> mTLS Provider -> HTTP Proxy -> Mock Server

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# PIDs for cleanup
MOCK_SERVER_PID=""
PROXY_PID=""

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"

    if [ -n "$MOCK_SERVER_PID" ] && kill -0 "$MOCK_SERVER_PID" 2>/dev/null; then
        echo "Stopping mock server (PID: $MOCK_SERVER_PID)"
        kill "$MOCK_SERVER_PID" 2>/dev/null || true
    fi

    if [ -n "$PROXY_PID" ] && kill -0 "$PROXY_PID" 2>/dev/null; then
        echo "Stopping proxy server (PID: $PROXY_PID)"
        kill "$PROXY_PID" 2>/dev/null || true
    fi

    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap for cleanup on exit
trap cleanup EXIT

# Print banner
echo -e "${BOLD}${CYAN}"
echo "========================================"
echo "    Integration Test Suite"
echo "========================================"
echo -e "${NC}"
echo "Testing: OpenCode -> mTLS Provider -> HTTP Proxy -> Mock Server"
echo ""

# Check prerequisites
echo -e "${BOLD}Checking prerequisites...${NC}"

# Check for certificates
if [ ! -f "$SCRIPT_DIR/certs/ca.crt" ]; then
    echo -e "${RED}Error: Certificates not found. Run: cd certs && ./generate.sh${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} Certificates found"

# Check for mock server binary
if [ ! -f "$SCRIPT_DIR/openai-mock-server/openai-mock-server" ]; then
    echo -e "${YELLOW}Building mock server...${NC}"
    (cd "$SCRIPT_DIR/openai-mock-server" && go build -o openai-mock-server .)
fi
echo -e "  ${GREEN}✓${NC} Mock server binary ready"

# Check for proxy binary
if [ ! -f "$SCRIPT_DIR/http-proxy/http-proxy" ]; then
    echo -e "${YELLOW}Building HTTP proxy...${NC}"
    (cd "$SCRIPT_DIR/http-proxy" && go build -o http-proxy .)
fi
echo -e "  ${GREEN}✓${NC} HTTP proxy binary ready"

# Check for mTLS provider
if [ ! -d "$SCRIPT_DIR/openai-mtls-provider/dist" ]; then
    echo -e "${YELLOW}Building mTLS provider...${NC}"
    (cd "$SCRIPT_DIR/openai-mtls-provider" && npm install && npm run build)
fi
echo -e "  ${GREEN}✓${NC} mTLS provider ready"

# Check for opencode
if ! command -v opencode &> /dev/null; then
    echo -e "${RED}Error: opencode not found in PATH${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} OpenCode installed"

echo ""

# Start mock server
echo -e "${BOLD}Starting mock server...${NC}"
cd "$SCRIPT_DIR/openai-mock-server"
./openai-mock-server > /tmp/mock-server.log 2>&1 &
MOCK_SERVER_PID=$!
sleep 1

if kill -0 "$MOCK_SERVER_PID" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} Mock server started (PID: $MOCK_SERVER_PID)"
else
    echo -e "  ${RED}✗${NC} Failed to start mock server"
    cat /tmp/mock-server.log
    exit 1
fi

# Start HTTP proxy
echo -e "${BOLD}Starting HTTP proxy...${NC}"
cd "$SCRIPT_DIR/http-proxy"
./http-proxy -verbose > /tmp/http-proxy.log 2>&1 &
PROXY_PID=$!
sleep 1

if kill -0 "$PROXY_PID" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} HTTP proxy started (PID: $PROXY_PID)"
else
    echo -e "  ${RED}✗${NC} Failed to start HTTP proxy"
    cat /tmp/http-proxy.log
    exit 1
fi

# Verify mock server is responding (direct connection with mTLS)
echo ""
echo -e "${BOLD}Verifying mock server (direct mTLS connection)...${NC}"
cd "$SCRIPT_DIR"
DIRECT_RESPONSE=$(curl -s --cacert certs/ca.crt --cert certs/client.crt --key certs/client.key https://localhost:8000/v1/models 2>&1 || echo "CURL_FAILED")

if echo "$DIRECT_RESPONSE" | grep -q "gpt-4o"; then
    echo -e "  ${GREEN}✓${NC} Mock server responding correctly"
else
    echo -e "  ${RED}✗${NC} Mock server not responding"
    echo "Response: $DIRECT_RESPONSE"
    exit 1
fi

# Test 1: OpenCode with mTLS provider through proxy
echo ""
echo -e "${BOLD}${CYAN}Test 1: OpenCode -> mTLS Provider -> Proxy -> Mock Server${NC}"
cd "$SCRIPT_DIR"

OPENCODE_RESPONSE=$(opencode run "Say hello in exactly 5 words" 2>&1)
OPENCODE_EXIT=$?

if [ $OPENCODE_EXIT -eq 0 ] && [ -n "$OPENCODE_RESPONSE" ]; then
    echo -e "  ${GREEN}✓${NC} OpenCode request successful"
    echo -e "  Response: ${CYAN}$OPENCODE_RESPONSE${NC}"
else
    echo -e "  ${RED}✗${NC} OpenCode request failed (exit code: $OPENCODE_EXIT)"
    echo "Response: $OPENCODE_RESPONSE"
    exit 1
fi

# Check proxy logs to verify request went through
echo ""
echo -e "${BOLD}Verifying proxy logs...${NC}"
if grep -q "CONNECT.*localhost:8000" /tmp/http-proxy.log; then
    TUNNEL_COUNT=$(grep -c "Tunnel established" /tmp/http-proxy.log)
    echo -e "  ${GREEN}✓${NC} Proxy handled $TUNNEL_COUNT CONNECT tunnel(s)"
else
    echo -e "  ${RED}✗${NC} No proxy tunnels found in logs"
    cat /tmp/http-proxy.log
    exit 1
fi

# Test 2: Run the Go test client through proxy
echo ""
echo -e "${BOLD}${CYAN}Test 2: Go Test Client -> Proxy -> Mock Server${NC}"
cd "$SCRIPT_DIR/openai-test-client"

if [ ! -f "./openai-test-client" ]; then
    echo -e "${YELLOW}Building test client...${NC}"
    go build -o openai-test-client .
fi

TEST_OUTPUT=$(./openai-test-client -proxy http://localhost:8080 2>&1)
TEST_EXIT=$?

PASSED=$(echo "$TEST_OUTPUT" | grep -o "Passed: [0-9]*" | grep -o "[0-9]*")
FAILED=$(echo "$TEST_OUTPUT" | grep -o "Failed: [0-9]*" | grep -o "[0-9]*")

if [ "$TEST_EXIT" -eq 0 ] && [ "$FAILED" = "0" ]; then
    echo -e "  ${GREEN}✓${NC} All $PASSED tests passed"
else
    echo -e "  ${RED}✗${NC} Tests failed: $FAILED failures"
    echo "$TEST_OUTPUT"
    exit 1
fi

# Summary
echo ""
echo -e "${BOLD}${CYAN}"
echo "========================================"
echo "    Integration Test Results"
echo "========================================"
echo -e "${NC}"
echo -e "${GREEN}${BOLD}All integration tests passed!${NC}"
echo ""
echo "Components tested:"
echo "  - OpenAI Mock Server (mTLS)"
echo "  - HTTP Proxy (CONNECT tunneling)"
echo "  - mTLS Provider (Bun/OpenCode)"
echo "  - Go Test Client (26 tests)"
echo ""
echo -e "${CYAN}Proxy statistics:${NC}"
FINAL_TUNNEL_COUNT=$(grep -c "Tunnel established" /tmp/http-proxy.log 2>/dev/null || echo "0")
echo "  Total CONNECT tunnels: $FINAL_TUNNEL_COUNT"
echo ""
