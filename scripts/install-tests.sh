#!/usr/bin/env bash
#
# OpsPilot Agent installer test harness.
#
# Runs scripts/install.sh against a mock Central server and stubbed system
# tools in a private sandbox, then asserts the required behaviors:
#
#   - successful registration (agent_id + secret persisted)
#   - registration response parsing (agent_id extracted from response)
#   - invalid token -> non-zero exit, service NOT started
#   - unreachable central -> non-zero exit, service NOT started
#   - invalid URL -> non-zero exit, service NOT started
#   - existing config preservation (server.* and extra sections kept)
#   - service started on success
#   - installer rerun (idempotent, no re-registration)
#   - already registered -> No  (skip registration, exit 0)
#   - already registered -> Yes (re-register, replace credentials)
#   - no duplicate registration unless the operator chooses to re-register
#   - agent_id persisted and used by heartbeat
#   - no secret printed, no token printed after registration
#
# Usage: scripts/install-tests.sh
#
# Requires: bash, python3, go (to build the mock agent ELF), curl.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly ROOT
INSTALL_SH="$ROOT/scripts/install.sh"
readonly INSTALL_SH

# --- sandbox -------------------------------------------------------------------

WORK="$(mktemp -d)"
MOCK_PID=""
cleanup() {
  [[ -n "$MOCK_PID" ]] && kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

SANDBOX="$WORK/sandbox"
mkdir -p "$SANDBOX/etc/opspilot" "$SANDBOX/usr/local/bin" \
  "$SANDBOX/etc/systemd/system" "$SANDBOX/stubs" "$SANDBOX/logs" "$SANDBOX/mock"

# A real ELF binary for OPSPILOT_LOCAL_BIN (the installer checks the ELF magic).
(cd "$ROOT" && GOOS=linux go build -o "$SANDBOX/mock/opspilot-agent-elf" ./cmd/agent)

# --- stubbed system tools ------------------------------------------------------

cat > "$SANDBOX/stubs/systemctl" <<'EOF'
#!/usr/bin/env bash
cmd="$1"; shift
printf '%s\n' "$cmd" >> "${SYSTEMCTL_LOG:?}"
case "$cmd" in
  daemon-reload|enable|start|stop|disable) exit 0 ;;
  is-active) echo "active"; exit 0 ;;
  status) echo "mock status: active"; exit 0 ;;
  *) exit 0 ;;
esac
EOF

cat > "$SANDBOX/stubs/journalctl" <<'EOF'
#!/usr/bin/env bash
echo "mock journal: no recent log lines"
exit 0
EOF

cat > "$SANDBOX/stubs/id" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  -u) echo "0"; exit 0 ;;
  opspilot) exit 0 ;;   # pretend the user already exists
  *) exit 1 ;;
esac
EOF

cat > "$SANDBOX/stubs/useradd" <<'EOF'
#!/usr/bin/env bash
printf 'useradd %s\n' "$*" >> "${USERADD_LOG:?}"
exit 0
EOF

cat > "$SANDBOX/stubs/hostname" <<'EOF'
#!/usr/bin/env bash
echo "test-host"
EOF

cat > "$SANDBOX/stubs/uname" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  -s) echo "Linux" ;;
  -m) echo "x86_64" ;;
  *) exit 0 ;;
esac
EOF

cat > "$SANDBOX/stubs/openssl" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "rand" ]]; then
  echo "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
  exit 0
fi
exit 0
EOF

chmod +x "$SANDBOX/stubs"/*

# --- mock central --------------------------------------------------------------

MOCK="$SANDBOX/mock/central.py"
cat > "$MOCK" <<'PYEOF'
import http.server, json, os, sys, uuid

VALID_TOKENS = os.environ.get("VALID_TOKENS", "ops_rt_valid").split(",")
STATE = {"registered": set(), "agent_id": None, "secret": None}
LOGFILE = os.environ.get("CENTRAL_LOG")

def log_line(line):
    if LOGFILE:
        with open(LOGFILE, "a") as f:
            f.write(line + "\n")

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            data = json.loads(raw)
        except Exception:
            data = {}

        if self.path == "/api/v1/agents/register":
            tok = data.get("registration_token", "")
            log_line("register token=%s" % tok)
            if tok not in VALID_TOKENS:
                self._send(401, {"error": {"code": "invalid_token",
                                           "message": "registration token invalid or expired"}})
                return
            if tok in STATE["registered"]:
                self._send(409, {"error": {"code": "token_already_used",
                                           "message": "registration token already used"}})
                return
            STATE["registered"].add(tok)
            STATE["agent_id"] = str(uuid.uuid4())
            STATE["secret"] = data.get("secret")
            log_line("registered agent_id=%s" % STATE["agent_id"])
            self._send(201, {"agent_id": STATE["agent_id"], "status": "offline"})
            return

        if self.path == "/api/v1/agents/heartbeat":
            ok = (data.get("agent_id") == STATE["agent_id"]
                  and data.get("secret") == STATE["secret"])
            if ok:
                self._send(200, {"status": "ok", "next_heartbeat": 30})
            else:
                self._send(401, {"error": {"code": "invalid_credentials",
                                           "message": "invalid agent credentials"}})
            return

        self._send(404, {"error": {"code": "not_found", "message": "not found"}})

    def _send(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

http.server.ThreadingHTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
PYEOF

start_mock() {
  # start_mock TOKENS -> starts a fresh mock central, sets MOCK_URL
  [[ -n "$MOCK_PID" ]] && kill "$MOCK_PID" 2>/dev/null || true
  MOCK_PID=""
  local port tokens="$1"
  port="$((20000 + RANDOM % 20000))"
  MOCK_URL="http://127.0.0.1:${port}"
  CENTRAL_LOG="$WORK/central.log"
  : > "$CENTRAL_LOG"
  PORT="$port" VALID_TOKENS="$tokens" CENTRAL_LOG="$CENTRAL_LOG" python3 "$MOCK" &
  MOCK_PID=$!
  for _ in $(seq 1 50); do
    if curl -sS -o /dev/null "$MOCK_URL/__probe__" 2>/dev/null; then break; fi
    sleep 0.1
  done
}

# --- helpers -------------------------------------------------------------------

CONFIG_DIR="$SANDBOX/etc/opspilot"
CONFIG="$CONFIG_DIR/agent.yaml"
BIN="$SANDBOX/usr/local/bin/opspilot-agent"
SERVICE="$SANDBOX/etc/systemd/system/opspilot-agent.service"
SYSTEMCTL_LOG="$SANDBOX/logs/systemctl.log"
USERADD_LOG="$SANDBOX/logs/useradd.log"

run_install() {
  # run_install INPUT -> runs install.sh feeding INPUT (printf %b escapes) to its prompts.
  local input="$1"
  : > "$SYSTEMCTL_LOG"; : > "$USERADD_LOG"
  PATH="$SANDBOX/stubs:$PATH" \
    OPSPILOT_CONFIG_DIR="$CONFIG_DIR" \
    OPSPILOT_BIN_PATH="$BIN" \
    OPSPILOT_SERVICE_PATH="$SERVICE" \
    OPSPILOT_LOCAL_BIN="$SANDBOX/mock/opspilot-agent-elf" \
    OPSPILOT_ALLOW_NON_ROOT=1 \
    SYSTEMCTL_LOG="$SYSTEMCTL_LOG" \
    USERADD_LOG="$USERADD_LOG" \
    bash "$INSTALL_SH" < <(printf '%b\n' "$input")
}

PASS=0; FAIL=0
pass() { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
fail() { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1" >&2; }
check() { # check DESC ACTUAL EXPECTED
  if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1 (got '$2', want '$3')"; fi
}
assert() { # assert DESC CMD...
  local desc="$1"; shift
  if "$@"; then
    pass "$desc"
  else
    fail "$desc"
  fi
}
refute() { # refute DESC CMD...: passes when CMD fails
  local desc="$1"; shift
  if "$@"; then fail "$desc"; else pass "$desc"; fi
}

agent_id_from() { sed -n 's/^agent_id: //p' "$1"; }

# --- scenarios ------------------------------------------------------------------

echo "== 1. successful registration =="
start_mock "ops_rt_valid"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
out="$(run_install "${MOCK_URL}
ops_rt_valid" 2>&1)"
rc=$?
check "register exits 0" "$rc" "0"
assert "agent_id persisted" test -s "$CONFIG" && grep -q '^agent_id: [0-9a-f-]' "$CONFIG"
assert "secret persisted" grep -q '^secret: 000102030405060708090a0b0c0d0e0f' "$CONFIG"
assert "registration_token persisted" grep -q '^registration_token: ops_rt_valid' "$CONFIG"
assert "central_url persisted" grep -q "^central_url: ${MOCK_URL}" "$CONFIG"
assert "service started on success" grep -q '^start$' "$SYSTEMCTL_LOG"
assert "service enabled on success" grep -q '^enable$' "$SYSTEMCTL_LOG"
assert "heartbeat verified on success" grep -q 'agent heartbeat verified' <<< "$out"
assert "agent registered logged" grep -q 'agent registered with id' <<< "$out"
assert "config file mode 0600" test "$(stat -f '%Lp' "$CONFIG")" = "600"

echo "== 2. registration response parsing =="
start_mock "ops_rt_parse"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
out="$(run_install "${MOCK_URL}
ops_rt_parse" 2>&1)"
rc=$?
check "register exits 0" "$rc" "0"
mock_id="$(sed -n 's/^registered agent_id=//p' "$CENTRAL_LOG" | head -n 1)"
persisted="$(agent_id_from "$CONFIG")"
check "agent_id matches response" "$persisted" "$mock_id"

echo "== 3. invalid token =="
start_mock "ops_rt_valid"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
rc=0
out="$(run_install "${MOCK_URL}
ops_rt_wrong" 2>&1)" || rc=$?
check "invalid token exits non-zero" "$rc" "1"
refute "invalid token: service NOT started" grep -q '^start$' "$SYSTEMCTL_LOG"
refute "invalid token: no agent_id written" grep -q '^agent_id: ' "$CONFIG" 2>/dev/null
assert "invalid token: error mentions token" grep -qi 'token' <<< "$out"

echo "== 4. unreachable central =="
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
rc=0
out="$(run_install "http://127.0.0.1:1
ops_rt_valid" 2>&1)" || rc=$?
check "unreachable central exits non-zero" "$rc" "1"
refute "unreachable: service NOT started" grep -q '^start$' "$SYSTEMCTL_LOG"
refute "unreachable: no agent_id written" grep -q '^agent_id: ' "$CONFIG" 2>/dev/null

echo "== 5. invalid URL =="
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
rc=0
out="$(run_install "not-a-url
ops_rt_valid" 2>&1)" || rc=$?
check "invalid URL exits non-zero" "$rc" "1"
refute "invalid URL: service NOT started" grep -q '^start$' "$SYSTEMCTL_LOG"

echo "== 6. existing config preservation =="
start_mock "ops_rt_preserve"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
cat > "$CONFIG" <<'EOF'
central_url: http://old.example:8080
registration_token: ops_rt_old
secret: old-secret
server:
  hostname: operator-host
  environment: staging
poll_interval: 10
projects:
  - name: backend
    repository: /srv/backend
EOF
chmod 0600 "$CONFIG"
run_install "${MOCK_URL}
ops_rt_preserve" >/dev/null 2>&1
assert "server.hostname preserved" grep -q '^  hostname: operator-host' "$CONFIG"
assert "server.environment preserved" grep -q '^  environment: staging' "$CONFIG"
assert "poll_interval preserved" grep -q '^poll_interval: 10' "$CONFIG"
assert "projects section preserved" grep -q '^projects:' "$CONFIG"
assert "agent_id added" grep -q '^agent_id: [0-9a-f-]' "$CONFIG"
assert "old token replaced" grep -q '^registration_token: ops_rt_preserve' "$CONFIG"
refute "old secret replaced" grep -q '^secret: old-secret' "$CONFIG"

echo "== 7. installer rerun (already registered -> No) =="
start_mock "ops_rt_rerun"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}
ops_rt_rerun" >/dev/null 2>&1
before_id="$(agent_id_from "$CONFIG")"
before_secret="$(sed -n 's/^secret: //p' "$CONFIG")"
before_token="$(sed -n 's/^registration_token: //p' "$CONFIG")"
before_calls="$(grep -c 'register token=' "$CENTRAL_LOG" || true)"

out="$(run_install "n" 2>&1)"
rc=$?
check "rerun (No) exits 0" "$rc" "0"
after_id="$(agent_id_from "$CONFIG")"
after_secret="$(sed -n 's/^secret: //p' "$CONFIG")"
after_token="$(sed -n 's/^registration_token: //p' "$CONFIG")"
after_calls="$(grep -c 'register token=' "$CENTRAL_LOG" || true)"
check "rerun (No): agent_id unchanged" "$after_id" "$before_id"
check "rerun (No): secret unchanged" "$after_secret" "$before_secret"
check "rerun (No): registration_token unchanged" "$after_token" "$before_token"
check "rerun (No): no extra register call" "$after_calls" "$before_calls"
assert "rerun (No): service started" grep -q '^start$' "$SYSTEMCTL_LOG"
assert "rerun (No): heartbeat verified" grep -q 'agent heartbeat verified' <<< "$out"

echo "== 8. already registered -> Yes (re-register) =="
start_mock "ops_rt_yes1,ops_rt_yes2"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}
ops_rt_yes1" >/dev/null 2>&1
before_id="$(agent_id_from "$CONFIG")"

# Answer Yes to re-register; empty Central URL (keeps existing); fresh token.
out="$(run_install "y
${MOCK_URL}
ops_rt_yes2" 2>&1)"
rc=$?
check "re-register (Yes) exits 0" "$rc" "0"
after_id="$(agent_id_from "$CONFIG")"
after_secret="$(sed -n 's/^secret: //p' "$CONFIG")"
after_token="$(sed -n 's/^registration_token: //p' "$CONFIG")"
assert "re-register: agent_id changed" test -n "$after_id" && test "$after_id" != "$before_id"
check "re-register: token replaced" "$after_token" "ops_rt_yes2"
check "re-register: secret regenerated" "$after_secret" "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
assert "re-register: heartbeat verified" grep -q 'agent heartbeat verified' <<< "$out"

echo "== 9. no secret printed / no token printed =="
start_mock "ops_rt_secret"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
out="$(run_install "${MOCK_URL}
ops_rt_secret" 2>&1)"
check "secret not printed" "$(grep -c '000102030405060708090a0b0c0d0e0f' <<< "$out" || true)" "0"
check "token not printed after registration" "$(grep -c 'ops_rt_secret' <<< "$out" || true)" "0"
assert "request body not left behind" test -z "$(find "$WORK" -name 'reg-body.json' -o -name 'hb-body.json')"

echo "== 10. no duplicate registration =="
start_mock "ops_rt_dup"
rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}
ops_rt_dup" >/dev/null 2>&1
calls_after_first="$(grep -c 'register token=' "$CENTRAL_LOG" || true)"
run_install "n" >/dev/null 2>&1
calls_after_second="$(grep -c 'register token=' "$CENTRAL_LOG" || true)"
check "no duplicate registration on rerun" "$calls_after_second" "$calls_after_first"

echo "== 11. input normalization (end-to-end) =="
start_mock "ops_rt_norm_cr,ops_rt_norm_sp,ops_rt_norm_ls,ops_rt_norm_slash,ops_rt_norm_tok"
config_url() { sed -n 's/^central_url: //p' "$CONFIG"; }
config_tok() { sed -n 's/^registration_token: //p' "$CONFIG"; }

rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}\r\nops_rt_norm_cr" >/dev/null 2>&1
check "trailing CR: url normalized" "$(config_url)" "$MOCK_URL"

rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}   \nops_rt_norm_sp" >/dev/null 2>&1
check "trailing spaces: url normalized" "$(config_url)" "$MOCK_URL"

rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "   ${MOCK_URL}\nops_rt_norm_ls" >/dev/null 2>&1
check "leading spaces: url normalized" "$(config_url)" "$MOCK_URL"

rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}/\nops_rt_norm_slash" >/dev/null 2>&1
check "trailing slash: url normalized" "$(config_url)" "$MOCK_URL"

rm -rf "$CONFIG_DIR"; mkdir -p "$CONFIG_DIR"
run_install "${MOCK_URL}\n  ops_rt_norm_tok  \n" >/dev/null 2>&1
check "token spaces: token normalized" "$(config_tok)" "ops_rt_norm_tok"

echo "== 12. normalize unit (extracted from install.sh) =="
normalize_unit() {
  local expr
  expr="$(sed -n '/^normalize_in()/,/^}/p;/^normalize_url()/,/^}/p' "$INSTALL_SH")"
  bash -c "
    $expr
    set -e
    normalize_in \$'url\r' o; [[ \"\$o\" == 'url' ]]
    normalize_in \$'url\n' o; [[ \"\$o\" == 'url' ]]
    normalize_in ' url ' o; [[ \"\$o\" == 'url' ]]
    normalize_in \$'  url\t \t' o; [[ \"\$o\" == 'url' ]]
    normalize_url 'http://host:9090/' o; [[ \"\$o\" == 'http://host:9090' ]]
    normalize_url \$'http://host:9090/\r' o; [[ \"\$o\" == 'http://host:9090' ]]
    normalize_url ' http://host:9090/ ' o; [[ \"\$o\" == 'http://host:9090' ]]
    echo UNIT_OK
  "
}
out="$(normalize_unit)"
check "normalize unit (CR/LF/spaces/slash)" "$out" "UNIT_OK"

echo
echo "results: ${PASS} passed, ${FAIL} failed"
[[ "$FAIL" -eq 0 ]] || exit 1
echo "ALL TESTS PASSED"
