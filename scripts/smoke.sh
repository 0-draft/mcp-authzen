#!/usr/bin/env bash
# End-to-end smoke test for mcp-authzen.
#
# 1. Starts a fake AuthZEN PDP (Python) on a free port that always returns
#    {"decision": true}.
# 2. Runs mcp-authzen pointed at it.
# 3. Drives an MCP initialize → tools/list → tools/call(authzen_evaluate)
#    sequence over stdio.
# 4. Asserts the decision was forwarded back to the caller.
#
# Exit codes:
#   0  decision propagated correctly
#   1  decision missing or wrong
#   2  protocol / setup failure

set -euo pipefail

BIN="${1:-./mcp-authzen}"
if [[ ! -x "$BIN" ]]; then
    echo "build first: go build ." >&2
    exit 2
fi

PORT=18181
PDP_LOG=$(mktemp)
# shellcheck disable=SC2329 # registered as EXIT trap below
cleanup() {
    if [[ -n "${PDP_PID:-}" ]] && kill -0 "$PDP_PID" 2>/dev/null; then
        kill "$PDP_PID" 2>/dev/null || true
    fi
    rm -f "$PDP_LOG"
}
trap cleanup EXIT

python3 - "$PORT" >"$PDP_LOG" 2>&1 <<'PY' &
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

class FakePDP(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        _ = self.rfile.read(length)
        body = json.dumps({"decision": True}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), FakePDP).serve_forever()
PY
PDP_PID=$!
disown "$PDP_PID" 2>/dev/null || true

# Wait for the fake PDP to be listening (pure-bash TCP probe)
for _ in {1..30}; do
    (echo >/dev/tcp/127.0.0.1/$PORT) 2>/dev/null && break
    sleep 0.1
done

OUT=$(printf '%s\n' \
        '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
        '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
        '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"authzen_evaluate","arguments":{"subject":"{\"type\":\"user\",\"id\":\"alice\"}","resource":"{\"type\":\"doc\",\"id\":\"d1\"}","action":"{\"name\":\"read\"}"}}}' \
    | AUTHZEN_PDP_URL="http://127.0.0.1:${PORT}/access/v1/evaluation" "$BIN")

DECISION=$(printf '%s\n' "$OUT" | tail -1 | jq -r '.result.content[0].text' | jq -r '.decision // empty')

case "$DECISION" in
    true)  echo "✓ smoke: decision=true forwarded from fake PDP"; exit 0 ;;
    false) echo "✗ smoke: decision=false (fake PDP returned true; mismatch)"; exit 1 ;;
    *)     echo "✗ smoke: no decision field in response. payload:"; printf '%s\n' "$OUT"; exit 2 ;;
esac
