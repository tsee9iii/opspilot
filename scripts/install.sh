#!/usr/bin/env bash
#
# OpsPilot Agent installer — Phase 2 (automatic registration)
#
# Downloads the latest released opspilot-agent binary from GitHub Releases,
# registers the agent with a Central server using the existing registration
# endpoint, persists the returned credentials (agent_id, signing_key) into
# /etc/opspilot/agent.yaml, installs it as a systemd service, and verifies by
# polling the heartbeat and lease endpoints with HMAC-signed requests.
#
# Authenticated agent requests are signed with the agent's per-agent signing
# key (issued by Central at registration) using the shared HMAC protocol in
# internal/agentsign: HMAC-SHA256 over
#   agent_id "\n" timestamp "\n" nonce "\n" method "\n" path "\n" body
# sent via the X-Agent-Id / X-Agent-Timestamp / X-Agent-Nonce /
# X-Agent-Signature headers. Requires openssl for the HMAC computation.
#
# The installation is idempotent: an already-registered agent is never
# overwritten unless the operator explicitly re-registers.
#
# Expected release assets per architecture (published on GitHub Releases):
#   opspilot-agent-linux-amd64
#   opspilot-agent-linux-arm64
#
# Usage: sudo scripts/install.sh
#
# Test overrides (documented, keep production defaults when unset):
#   OPSPILOT_CONFIG_DIR     config directory   (default /etc/opspilot)
#   OPSPILOT_BIN_PATH       binary path        (default /usr/local/bin/opspilot-agent)
#   OPSPILOT_SERVICE_PATH   unit path          (default /etc/systemd/system/opspilot-agent.service)
#   OPSPILOT_LOCAL_BIN      use a local binary file instead of downloading
#   OPSPILOT_ALLOW_NON_ROOT set to 1 to skip the root check (testing only)
#   OPSPILOT_DEBUG          set to 1 to print the normalized central URL

set -euo pipefail

readonly REPO="tsee9iii/opspilot"
readonly CONFIG_DIR="${OPSPILOT_CONFIG_DIR:-/etc/opspilot}"
readonly CONFIG_PATH="${CONFIG_DIR}/agent.yaml"
readonly BIN_PATH="${OPSPILOT_BIN_PATH:-/usr/local/bin/opspilot-agent}"
readonly SERVICE_PATH="${OPSPILOT_SERVICE_PATH:-/etc/systemd/system/opspilot-agent.service}"
readonly SERVICE_NAME="opspilot-agent"

log() { printf '[installer] %s\n' "$*"; }
die() { printf '[installer] error: %s\n' "$*" >&2; exit 1; }
debug() { [[ "${OPSPILOT_DEBUG:-0}" == "1" ]] && printf '[installer] %s\n' "$*" >&2 || true; }

# normalize_in STRING OUTPUT -> strip leading/trailing whitespace and a trailing
# CR/LF, storing the result in variable OUTPUT. Uses printf -v (no subshell), so
# it is safe to call between reads of a shared stdin pipe.
normalize_in() {
  local s="$1" out="$2"
  s="${s%$'\r'}"
  s="${s%$'\n'}"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf -v "$out" '%s' "$s"
}

# normalize_url STRING OUTPUT -> as normalize_in plus strip trailing slashes.
normalize_url() {
  local s="$1" out="$2" n
  normalize_in "$s" n
  s="$n"
  while [[ "$s" == */ ]]; do s="${s%/}"; done
  printf -v "$out" '%s' "$s"
}

# yaml_get_top KEY -> value of a top-level `key: value` line (quotes stripped).
yaml_get_top() {
  [[ -f "$CONFIG_PATH" ]] && sed -n "s/^${1}[[:space:]]*:[[:space:]]*\"\{0,1\}\([^\"]*\)\"\{0,1\}[[:space:]]*$/\1/p" "$CONFIG_PATH" | head -n 1 || true
}

# yaml_get_nested PARENT KEY -> value of a `  key: value` line under PARENT.
yaml_get_nested() {
  [[ -f "$CONFIG_PATH" ]] || return 0
  awk -v p="${1}:" -v k="${2}" '
    $0 ~ "^" p { inb = 1; next }
    inb && $0 !~ /^[[:space:]]/ { exit }
    inb && $0 ~ "^[[:space:]]*" k "[[:space:]]*:" {
      sub("^[[:space:]]*" k "[[:space:]]*:[[:space:]]*", "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' "$CONFIG_PATH"
}

# sed_escape STRING -> escapes sed replacement-special characters (& \ and |).
sed_escape() { printf '%s' "$1" | sed 's/[&|\\]/\\&/g'; }

# yaml_set_top KEY VALUE -> update the top-level line or append it.
yaml_set_top() {
  local key="$1" value="$2" tmp="$workdir/config.new"
  if grep -qE "^${key}:" "$CONFIG_PATH"; then
    sed -E "s|^(${key}:).*|\\1 $(sed_escape "$value")|" "$CONFIG_PATH" > "$tmp"
  else
    { cat "$CONFIG_PATH"; printf '%s: %s\n' "$key" "$value"; } > "$tmp"
  fi
  mv -f "$tmp" "$CONFIG_PATH"
  chmod 0600 "$CONFIG_PATH"
}

# yaml_set_nested PARENT KEY VALUE -> update the nested line or insert it under PARENT.
yaml_set_nested() {
  local parent="$1" key="$2" value="$3" tmp="$workdir/config.new"
  if grep -qE "^  ${key}:" "$CONFIG_PATH"; then
    sed -E "s|^(  ${key}:).*|\\1 $(sed_escape "$value")|" "$CONFIG_PATH" > "$tmp"
  elif grep -qE "^${parent}:" "$CONFIG_PATH"; then
    sed -E "s|^(${parent}:.*)$|\\1\n  ${key}: $(sed_escape "$value")|" "$CONFIG_PATH" > "$tmp"
  else
    { cat "$CONFIG_PATH"; printf '\n%s:\n  %s: %s\n' "$parent" "$key" "$value"; } > "$tmp"
  fi
  mv -f "$tmp" "$CONFIG_PATH"
  chmod 0600 "$CONFIG_PATH"
}

# json_quote STRING -> a JSON string literal (escapes backslash and quote).
json_quote() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# rand_hex NBYTES -> prints N random bytes as lowercase hex.
rand_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$1"
  else
    od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
  fi
}

# http_code OUT_FILE URL BODY_FILE [HEADER...] -> prints the HTTP status to
# stdout. Uses --data-binary @file so secrets never appear in argv.
http_code() {
  local out="$1" url="$2" body="$3" h
  shift 3
  local args=(-sS -o "$out" -w '%{http_code}' -X POST "$url" \
    -H 'Content-Type: application/json' --data-binary "@$body")
  for h in "$@"; do args+=(-H "$h"); done
  curl "${args[@]}"
}

# echo body to file without exposing it in the command line.
req_file() { printf '%s' "$1" > "$2"; chmod 0600 "$2"; }

# sign_agent_request AGENT_ID SIGNING_KEY METHOD PATH BODY_FILE
# Computes the HMAC request-signing headers for a single agent request,
# following internal/agentsign exactly (canonical string, header names,
# Unix-second timestamp, 16-byte hex nonce). Results are exported through the
# global AGENT_TS / AGENT_NONCE / AGENT_SIGNATURE variables. BODY_FILE must be
# a file (not a here-string) so the signed bytes match the bytes curl sends.
#
# The signing key is used as its literal string bytes (HMAC key), matching the
# Go agent's agentsign.Sign. Requires openssl for the HMAC-SHA256 computation.
sign_agent_request() {
  local agent_id="$1" key="$2" method="$3" path="$4" body_file="$5" body canonical
  command -v openssl >/dev/null 2>&1 || die "openssl is required for HMAC request signing"
  body="$(<"$body_file")"
  AGENT_TS="$(date +%s)"
  AGENT_NONCE="$(rand_hex 16)"
  canonical="$(printf '%s\n%s\n%s\n%s\n%s\n%s' \
    "$agent_id" "$AGENT_TS" "$AGENT_NONCE" "$method" "$path" "$body")"
  AGENT_SIGNATURE="$(printf '%s' "$canonical" | openssl dgst -sha256 -mac HMAC -macopt "key:$key" 2>/dev/null | awk '{print $NF}')"
  [[ -n "$AGENT_SIGNATURE" ]] || die "failed to compute request signature (is openssl working?)"
}

# agent_post OUT_FILE URL BODY_FILE METHOD PATH -> signs a request with the
# current agent identity (global AGENT_ID/SIGNING_KEY) and POSTs it, printing
# the HTTP status code. Intended for `code="$(agent_post ... || true)"`.
# The signature covers method + path + body exactly, matching the production
# agent.
agent_post() {
  local out="$1" url="$2" body_file="$3" method="$4" path="$5"
  sign_agent_request "$AGENT_ID" "$SIGNING_KEY" "$method" "$path" "$body_file"
  http_code "$out" "$url" "$body_file" \
    "X-Agent-Id: ${AGENT_ID}" \
    "X-Agent-Timestamp: ${AGENT_TS}" \
    "X-Agent-Nonce: ${AGENT_NONCE}" \
    "X-Agent-Signature: ${AGENT_SIGNATURE}"
}

# --- platform detection -------------------------------------------------------

case "$(uname -s)" in
  Linux) ;;
  *) die "unsupported OS: $(uname -s) (only Linux is supported)" ;;
esac

case "$(uname -m)" in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) die "unsupported architecture: $(uname -m) (only amd64 and arm64 are supported)" ;;
esac
readonly ARCH

log "platform: linux/${ARCH}"

if [[ "${OPSPILOT_ALLOW_NON_ROOT:-0}" != "1" ]]; then
  [[ "$(id -u)" -eq 0 ]] || die "run as root (sudo)"
fi

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v systemctl >/dev/null 2>&1 || die "systemd (systemctl) is required"

# --- obtain the release binary ------------------------------------------------

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

if [[ -n "${OPSPILOT_LOCAL_BIN:-}" ]]; then
  log "using local binary ${OPSPILOT_LOCAL_BIN}"
  cp "$OPSPILOT_LOCAL_BIN" "${workdir}/opspilot-agent"
else
  asset_url="https://github.com/${REPO}/releases/latest/download/opspilot-agent-linux-${ARCH}"
  log "downloading ${asset_url}"
  curl -fsSL --retry 3 -o "${workdir}/opspilot-agent" "$asset_url"
fi

[[ -s "${workdir}/opspilot-agent" ]] || die "downloaded binary is empty"
if [[ "$(head -c 4 "${workdir}/opspilot-agent")" != $'\x7fELF' ]]; then
  die "downloaded file is not a Linux ELF binary (is the asset opspilot-agent-linux-${ARCH} published?)"
fi
chmod 0755 "${workdir}/opspilot-agent"

# --- install the binary -------------------------------------------------------

install -m 0755 "${workdir}/opspilot-agent" "$BIN_PATH"
log "installed ${BIN_PATH}"

# --- config directory ---------------------------------------------------------

mkdir -p "$CONFIG_DIR"

# --- opspilot system user -----------------------------------------------------

if ! id "opspilot" >/dev/null 2>&1; then
  log "creating system user opspilot"
  useradd --system --no-create-home --user-group --shell /bin/false opspilot
fi

# --- registration decision ----------------------------------------------------

REGISTER=true
if [[ -f "$CONFIG_PATH" ]]; then
  existing_agent_id="$(yaml_get_top agent_id)"
  existing_secret="$(yaml_get_top secret)"
  if [[ -n "$existing_agent_id" && -n "$existing_secret" ]]; then
    printf 'Agent is already registered (agent_id=%s).\n' "$existing_agent_id"
    printf 'Re-register this agent? (y/N): '
    IFS= read -r ans || ans="n"
    case "${ans:-n}" in
      y|Y|yes|Yes|YES) REGISTER=true ;;
      *) REGISTER=false ;;
    esac
  fi
fi

if [[ "$REGISTER" == true ]]; then
  existing_central="$(yaml_get_top central_url)"
  normalize_url "$existing_central" existing_central
  existing_token="$(yaml_get_top registration_token)"
  normalize_in "$existing_token" existing_token
  existing_hostname="$(yaml_get_nested server hostname)"
  existing_environment="$(yaml_get_nested server environment)"
  existing_version="$(yaml_get_top version)"
  already_registered="$(yaml_get_top agent_id)"

  # --- prompt for Central URL -------------------------------------------------
  CENTRAL_URL=""
  while [[ -z "$CENTRAL_URL" ]]; do
    if [[ -n "$existing_central" ]]; then
      printf 'Central URL [%s]: ' "$existing_central"
    else
      printf 'Central URL: '
    fi
    IFS= read -r v
    [[ -n "$v" ]] || v="$existing_central"
    normalize_url "$v" v
    if [[ -n "$v" ]]; then CENTRAL_URL="$v"; else printf '[installer] Central URL cannot be empty\n' >&2; fi
  done
  debug "central URL: ${CENTRAL_URL}"

  # --- prompt for Registration Token ------------------------------------------
  REGISTRATION_TOKEN=""
  # On a re-register the previous token is consumed/rotated, so never reuse it.
  token_default=""
  [[ -z "$already_registered" ]] && token_default="$existing_token"
  while [[ -z "$REGISTRATION_TOKEN" ]]; do
    printf 'Registration Token: '
    IFS= read -r v
    [[ -n "$v" ]] || v="$token_default"
    normalize_in "$v" v
    if [[ -n "$v" ]]; then REGISTRATION_TOKEN="$v"; else printf '[installer] Registration Token cannot be empty\n' >&2; fi
  done

  HOSTNAME="${existing_hostname:-$(hostname)}"
  ENVIRONMENT="${existing_environment:-production}"
  VERSION="${existing_version:-0.1.0}"

  # --- generate a cryptographically secure secret ------------------------------
  SECRET="$(rand_hex 32)"

  # --- register with Central ----------------------------------------------------
  register_url="${CENTRAL_URL%/}/api/v1/agents/register"
  log "registering agent"
  debug "registering with ${CENTRAL_URL}"
  req_file "$(printf '{"registration_token":"%s","secret":"%s","version":"%s","server":{"hostname":"%s","environment":"%s"}}' \
    "$(json_quote "$REGISTRATION_TOKEN")" "$(json_quote "$SECRET")" "$(json_quote "$VERSION")" \
    "$(json_quote "$HOSTNAME")" "$(json_quote "$ENVIRONMENT")")" "$workdir/reg-body.json"

  code="$(http_code "$workdir/reg.json" "$register_url" "$workdir/reg-body.json" || true)"
  if [[ "$code" != "201" ]]; then
    errmsg="$(sed -n 's/.*"message"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$workdir/reg.json" | head -n 1 || true)"
    die "registration failed (HTTP ${code}): ${errmsg:-cannot reach central or invalid response}"
  fi

  AGENT_ID="$(sed -n 's/.*"agent_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$workdir/reg.json" | head -n 1)"
  [[ -n "$AGENT_ID" ]] || die "registration response did not include agent_id"

  SIGNING_KEY="$(sed -n 's/.*"signing_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$workdir/reg.json" | head -n 1)"
  [[ -n "$SIGNING_KEY" ]] || die "registration response did not include signing_key"

  # --- persist config (installer-owned fields only) ------------------------------
  if [[ ! -f "$CONFIG_PATH" ]]; then
    log "creating ${CONFIG_PATH}"
    req_file "central_url: ${CENTRAL_URL}
registration_token: ${REGISTRATION_TOKEN}
secret: ${SECRET}
signing_key: ${SIGNING_KEY}
version: ${VERSION}
server:
  hostname: ${HOSTNAME}
  environment: ${ENVIRONMENT}
agent_id: ${AGENT_ID}
poll_interval: 5
" "$CONFIG_PATH"
    chmod 0600 "$CONFIG_PATH"
  else
    yaml_set_top central_url "$CENTRAL_URL"
    yaml_set_top registration_token "$REGISTRATION_TOKEN"
    yaml_set_top secret "$SECRET"
    yaml_set_top signing_key "$SIGNING_KEY"
    yaml_set_top agent_id "$AGENT_ID"
    yaml_set_top version "$VERSION"
    [[ -n "$(yaml_get_nested server hostname)" ]] || yaml_set_nested server hostname "$HOSTNAME"
    [[ -n "$(yaml_get_nested server environment)" ]] || yaml_set_nested server environment "$ENVIRONMENT"
  fi
  chown opspilot:opspilot "$CONFIG_DIR" "$CONFIG_PATH" || true
  log "agent registered with id ${AGENT_ID}"
else
  log "agent already registered; skipping registration (agent_id and secret preserved)"
  CENTRAL_URL="$(yaml_get_top central_url)"
  AGENT_ID="$(yaml_get_top agent_id)"
  SECRET="$(yaml_get_top secret)"
  SIGNING_KEY="$(yaml_get_top signing_key)"
fi

chown opspilot:opspilot "$CONFIG_DIR" "$CONFIG_PATH" || true

# --- systemd service ------------------------------------------------------------

log "installing ${SERVICE_PATH}"
cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=OpsPilot Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${BIN_PATH}
Restart=always
RestartSec=5
User=opspilot
Group=opspilot
WorkingDirectory=/etc/opspilot

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$SERVICE_PATH"

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"
log "opspilot-agent service enabled and started"

# --- verification ---------------------------------------------------------------

if [[ -n "${CENTRAL_URL:-}" && -n "${AGENT_ID:-}" && -n "${SIGNING_KEY:-}" ]]; then
  log "verifying agent authentication (HMAC signing)"
  hb_url="${CENTRAL_URL%/}/api/v1/agents/heartbeat"
  req_file "$(printf '{"agent_id":"%s"}' "$(json_quote "$AGENT_ID")")" "$workdir/hb-body.json"
  lease_url="${CENTRAL_URL%/}/api/v1/commands/lease"
  req_file "$(printf '{"agent_id":"%s"}' "$(json_quote "$AGENT_ID")")" "$workdir/lease-body.json"

  healthy=0
  for _ in 1 2 3 4 5; do
    hb_code="$(agent_post "$workdir/hb.json" "$hb_url" "$workdir/hb-body.json" "POST" "/api/v1/agents/heartbeat" || true)"
    if [[ "$hb_code" == "200" ]]; then healthy=1; break; fi
    sleep 1
  done
  if [[ "$healthy" == "1" ]]; then
    log "agent heartbeat verified"
    lease_code="$(agent_post "$workdir/lease.json" "$lease_url" "$workdir/lease-body.json" "POST" "/api/v1/commands/lease" || true)"
    if [[ "$lease_code" == "200" || "$lease_code" == "204" ]]; then
      log "agent lease verified"
    else
      log "warning: heartbeat verified but lease request returned HTTP ${lease_code}"
    fi
  else
    echo "---- systemctl status opspilot-agent ----" >&2
    systemctl status "$SERVICE_NAME" --no-pager 2>&1 || true
    echo "---- journalctl -u opspilot-agent ----" >&2
    journalctl -u "$SERVICE_NAME" -n 50 --no-pager 2>&1 || true
    if [[ "$REGISTER" == true ]]; then
      die "agent did not become healthy after install; see status above"
    else
      log "warning: agent did not confirm heartbeat; see status above (existing agent left as-is)"
    fi
  fi
elif [[ -n "${CENTRAL_URL:-}" && -n "${AGENT_ID:-}" ]]; then
  log "warning: no signing_key in config; skipping HMAC verification (re-register the agent to obtain a signing key)"
else
  log "skipping verification (no central_url/agent_id in config)"
fi

cat <<'EOF'

Installation completed.
Agent successfully registered.
EOF