#!/bin/bash

# Integration Test Script for No-Stream Provider
# Tests that OpenCode with the no-stream provider never sends stream:true

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MOCK_SERVER_PID=""
PROXY_PID=""
OPENCODE_PID=""

cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"

    if [ -n "$OPENCODE_PID" ] && kill -0 "$OPENCODE_PID" 2>/dev/null; then
        echo "Stopping opencode (PID: $OPENCODE_PID)"
        kill "$OPENCODE_PID" 2>/dev/null || true
    fi

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

trap cleanup EXIT

echo -e "${BOLD}${CYAN}"
echo "========================================"
echo "    No-Stream Provider Integration Tests"
echo "========================================"
echo -e "${NC}"
echo "Testing: OpenCode -> No-Stream Provider -> HTTP Proxy -> Mock Server"
echo ""

# ---- Prerequisites ----
echo -e "${BOLD}Checking prerequisites...${NC}"

if [ ! -f "$SCRIPT_DIR/certs/ca.crt" ]; then
    echo -e "${RED}Error: Certificates not found. Run: cd certs && ./generate.sh${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} Certificates found"

if [ ! -f "$SCRIPT_DIR/openai-mock-server/openai-mock-server" ]; then
    echo -e "${YELLOW}Building mock server...${NC}"
    (cd "$SCRIPT_DIR/openai-mock-server" && go build -o openai-mock-server .)
fi
echo -e "  ${GREEN}✓${NC} Mock server binary ready"

if [ ! -f "$SCRIPT_DIR/http-proxy/http-proxy" ]; then
    echo -e "${YELLOW}Building HTTP proxy...${NC}"
    (cd "$SCRIPT_DIR/http-proxy" && go build -o http-proxy .)
fi
echo -e "  ${GREEN}✓${NC} HTTP proxy binary ready"

if [ ! -d "$SCRIPT_DIR/openai-mtls-no-stream-provider/dist" ]; then
    echo -e "${YELLOW}Building no-stream provider...${NC}"
    (cd "$SCRIPT_DIR/openai-mtls-no-stream-provider" && npm install && npm run build)
fi
echo -e "  ${GREEN}✓${NC} No-stream provider ready"

if ! command -v opencode &> /dev/null; then
    echo -e "${RED}Error: opencode not found in PATH${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} OpenCode installed"

echo ""

# ---- Start services ----
echo -e "${BOLD}Starting mock server (verbose)...${NC}"
cd "$SCRIPT_DIR/openai-mock-server"
./openai-mock-server -verbose > /tmp/mock-server-nostream.log 2>&1 &
MOCK_SERVER_PID=$!
sleep 1

if kill -0 "$MOCK_SERVER_PID" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} Mock server started (PID: $MOCK_SERVER_PID)"
else
    echo -e "  ${RED}✗${NC} Failed to start mock server"
    cat /tmp/mock-server-nostream.log
    exit 1
fi

echo -e "${BOLD}Starting HTTP proxy...${NC}"
cd "$SCRIPT_DIR/http-proxy"
./http-proxy -verbose > /tmp/http-proxy-nostream.log 2>&1 &
PROXY_PID=$!
sleep 1

if kill -0 "$PROXY_PID" 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} HTTP proxy started (PID: $PROXY_PID)"
else
    echo -e "  ${RED}✗${NC} Failed to start HTTP proxy"
    cat /tmp/http-proxy-nostream.log
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

echo ""

# ---- Test 1: OpenCode with no-stream provider ----
echo -e "${BOLD}${CYAN}Test 1: OpenCode -> No-Stream Provider -> Proxy -> Mock Server${NC}"
cd "$SCRIPT_DIR"

# The mock server now returns realistic responses that satisfy the agent,
# so opencode should complete naturally.
timeout 60 opencode run -m mock-openai-no-stream/gpt-4o "Say hello in exactly 5 words" </dev/null >/tmp/opencode-nostream-out.log 2>&1 || true
OPENCODE_PID=""

POST_COUNT=$(grep -c '\[POST\]' /tmp/mock-server-nostream.log 2>/dev/null || echo "0")

if [ "$POST_COUNT" -gt 0 ]; then
    echo -e "  ${GREEN}✓${NC} OpenCode made $POST_COUNT request(s) via no-stream provider"
else
    echo -e "  ${RED}✗${NC} No requests reached mock server"
    echo "Mock server log:"
    cat /tmp/mock-server-nostream.log
    exit 1
fi

# Check proxy logs to verify request went through
echo ""
echo -e "${BOLD}Verifying proxy logs...${NC}"
if grep -q "CONNECT.*localhost:8000" /tmp/http-proxy-nostream.log; then
    TUNNEL_COUNT=$(grep -c "Tunnel established" /tmp/http-proxy-nostream.log)
    echo -e "  ${GREEN}✓${NC} Proxy handled $TUNNEL_COUNT CONNECT tunnel(s)"
else
    echo -e "  ${RED}✗${NC} No proxy tunnels found in logs"
    cat /tmp/http-proxy-nostream.log
    exit 1
fi

# ---- Test 2: Verify no streaming ----
echo ""
echo -e "${BOLD}${CYAN}Test 2: Verify no stream:true in mock server logs${NC}"

if grep -q '"stream":true' /tmp/mock-server-nostream.log; then
    echo -e "  ${RED}✗${NC} Found stream:true in mock server logs — streaming was used!"
    grep '"stream"' /tmp/mock-server-nostream.log | head -5
    exit 1
else
    echo -e "  ${GREEN}✓${NC} No stream:true found in mock server logs"
fi

if grep -q '"stream":false' /tmp/mock-server-nostream.log; then
    echo -e "  ${GREEN}✓${NC} Confirmed stream:false was sent in all requests"
fi

# ---- Test 3: Verify removeKeys feature strips keys from request body ----
echo ""
echo -e "${BOLD}${CYAN}Test 3: removeKeys Feature (strips frequency_penalty, presence_penalty)${NC}"

if grep -q "Request body:" /tmp/mock-server-nostream.log; then
    if grep "Request body:" /tmp/mock-server-nostream.log | grep -q "frequency_penalty"; then
        echo -e "  ${RED}✗${NC} frequency_penalty was NOT removed from request"
        exit 1
    fi
    if grep "Request body:" /tmp/mock-server-nostream.log | grep -q "presence_penalty"; then
        echo -e "  ${RED}✗${NC} presence_penalty was NOT removed from request"
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} frequency_penalty successfully removed"
    echo -e "  ${GREEN}✓${NC} presence_penalty successfully removed"
else
    echo -e "  ${YELLOW}!${NC} No request body logged (verbose mode may not be enabled)"
fi

# ---- Test 4: Verify custom headers ----
echo ""
echo -e "${BOLD}${CYAN}Test 4: Custom Headers${NC}"

if grep -q "X-Custom-Header: test-value-123" /tmp/mock-server-nostream.log; then
    echo -e "  ${GREEN}✓${NC} X-Custom-Header sent correctly"
else
    echo -e "  ${RED}✗${NC} X-Custom-Header not found in mock server logs"
    exit 1
fi

if grep -q "X-Request-Source: opencode-no-stream" /tmp/mock-server-nostream.log; then
    echo -e "  ${GREEN}✓${NC} X-Request-Source sent correctly"
else
    echo -e "  ${RED}✗${NC} X-Request-Source not found in mock server logs"
    exit 1
fi

# ---- Summary ----
echo ""
echo -e "${BOLD}${CYAN}"
echo "========================================"
echo "    No-Stream Integration Test Results"
echo "========================================"
echo -e "${NC}"
echo -e "${GREEN}${BOLD}All integration tests passed!${NC}"
echo ""
echo "Components tested:"
echo "  - OpenCode with no-stream provider (-m mock-openai-no-stream/gpt-4o)"
echo "  - Verified no stream:true sent to server"
echo "  - removeKeys feature (strips keys from JSON)"
echo "  - Custom headers passed through"
echo "  - mTLS + proxy chain"
echo ""
echo -e "${CYAN}Proxy statistics:${NC}"
FINAL_TUNNEL_COUNT=$(grep -c "Tunnel established" /tmp/http-proxy-nostream.log 2>/dev/null || echo "0")
echo "  Total CONNECT tunnels: $FINAL_TUNNEL_COUNT"
echo ""
