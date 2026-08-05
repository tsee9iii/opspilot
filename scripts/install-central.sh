#!/usr/bin/env bash
#
# OpsPilot Central installer — Phase 1
#
# Downloads the latest released opspilot-central binary from GitHub Releases and
# installs it as a systemd service on a Linux (amd64/arm64) host.
#
# This installer does NOT install PostgreSQL, does NOT create a database, does
# NOT run migrations, and does NOT generate registration tokens.
#
# Expected release assets per architecture (published on GitHub Releases):
#   opspilot-central-linux-amd64
#   opspilot-central-linux-arm64
#
# Usage: sudo scripts/install-central.sh

set -euo pipefail

readonly REPO="tsee9iii/opspilot"
readonly BIN_PATH="/usr/local/bin/opspilot-central"
readonly SERVICE_PATH="/etc/systemd/system/opspilot-central.service"
readonly CONFIG_DIR="/etc/opspilot"
readonly CONFIG_PATH="/etc/opspilot/central.yaml"
readonly SERVICE_NAME="opspilot-central"
readonly HEALTH_URL="http://127.0.0.1:8080/health"

log() { printf '[installer] %s\n' "$*"; }
die() { printf '[installer] error: %s\n' "$*" >&2; exit 1; }

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

[[ "$(id -u)" -eq 0 ]] || die "run as root (sudo)"

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v systemctl >/dev/null 2>&1 || die "systemd (systemctl) is required"

# --- download the latest release binary --------------------------------------

asset_url="https://github.com/${REPO}/releases/latest/download/opspilot-central-linux-${ARCH}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

log "downloading ${asset_url}"
curl -fsSL --retry 3 -o "${workdir}/opspilot-central" "$asset_url"

[[ -s "${workdir}/opspilot-central" ]] || die "downloaded binary is empty"
if [[ "$(head -c 4 "${workdir}/opspilot-central")" != $'\x7fELF' ]]; then
  die "downloaded file is not a Linux ELF binary (is the asset opspilot-central-linux-${ARCH} published?)"
fi
chmod 0755 "${workdir}/opspilot-central"

# --- install the binary -------------------------------------------------------

install -m 0755 "${workdir}/opspilot-central" "$BIN_PATH"
log "installed ${BIN_PATH}"

# --- config directory ----------------------------------------------------------

mkdir -p "$CONFIG_DIR"

# --- opspilot system user -------------------------------------------------------

if ! id "opspilot" >/dev/null 2>&1; then
  log "creating system user opspilot"
  useradd --system --no-create-home --user-group --shell /bin/false opspilot
fi

# --- config template (created only when missing) ------------------------------

if [[ ! -f "$CONFIG_PATH" ]]; then
  log "creating ${CONFIG_PATH}"
  cat > "$CONFIG_PATH" <<'EOF'
server:
  host: 0.0.0.0
  port: 8080

database:
  host:
  port:
  database:
  username:
  password:
EOF
  chmod 0600 "$CONFIG_PATH"
fi
chown opspilot:opspilot "$CONFIG_DIR" "$CONFIG_PATH"

# --- systemd service ------------------------------------------------------------

log "installing ${SERVICE_PATH}"
cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=OpsPilot Central
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${BIN_PATH}
Restart=always
RestartSec=5
User=opspilot
Group=opspilot
WorkingDirectory=${CONFIG_DIR}

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$SERVICE_PATH"

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"
log "opspilot-central service enabled and started"

# --- health check (warn only, never fail the install) ---------------------------

log "waiting up to 15s for ${HEALTH_URL}"
healthy=0
for _ in $(seq 1 15); do
  if curl -fsS --max-time 2 -o /dev/null "$HEALTH_URL"; then
    healthy=1
    break
  fi
  sleep 1
done

if [[ "$healthy" -eq 1 ]]; then
  log "central is healthy"
else
  log "warning: central is not responding at ${HEALTH_URL} yet — it may not start until PostgreSQL is configured"
fi

# --- PostgreSQL verification (verify only) ---------------------------------------

if command -v psql >/dev/null 2>&1; then
  log "PostgreSQL client (psql) is available"
else
  log "PostgreSQL is not installed."
  log "Central may not start until PostgreSQL is configured."
fi

cat <<'EOF'

Installation completed.

Next steps:
1. Edit /etc/opspilot/central.yaml

2. Ensure PostgreSQL is available

3. Restart:

sudo systemctl restart opspilot-central

4. Verify:

systemctl status opspilot-central
EOF
