#!/usr/bin/env bash
#
# OpsPilot Agent installer — Phase 1
#
# Downloads the latest released opspilot-agent binary from GitHub Releases and
# installs it as a systemd service on a Linux (amd64/arm64) host.
#
# This installer does NOT register the agent and does NOT contact the Central
# server. The config template is created for the operator to fill in, after
# which the service must be restarted.
#
# Expected release assets per architecture (published on GitHub Releases):
#   opspilot-agent-linux-amd64
#   opspilot-agent-linux-arm64
#
# Usage: sudo scripts/install.sh

set -euo pipefail

readonly REPO="tsee9iii/opspilot"
readonly BIN_PATH="/usr/local/bin/opspilot-agent"
readonly SERVICE_PATH="/etc/systemd/system/opspilot-agent.service"
readonly CONFIG_DIR="/etc/opspilot"
readonly CONFIG_PATH="/etc/opspilot/agent.yaml"
readonly SERVICE_NAME="opspilot-agent"

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

asset_url="https://github.com/${REPO}/releases/latest/download/opspilot-agent-linux-${ARCH}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

log "downloading ${asset_url}"
curl -fsSL --retry 3 -o "${workdir}/opspilot-agent" "$asset_url"

[[ -s "${workdir}/opspilot-agent" ]] || die "downloaded binary is empty"
if [[ "$(head -c 4 "${workdir}/opspilot-agent")" != $'\x7fELF' ]]; then
  die "downloaded file is not a Linux ELF binary (is the asset opspilot-agent-linux-${ARCH} published?)"
fi
chmod 0755 "${workdir}/opspilot-agent"

# --- install the binary -------------------------------------------------------

install -m 0755 "${workdir}/opspilot-agent" "$BIN_PATH"
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
central_url:
registration_token:
secret:
server:
  hostname:
  environment:
EOF
  chmod 0600 "$CONFIG_PATH"
fi
chown opspilot:opspilot "$CONFIG_DIR" "$CONFIG_PATH"

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

if systemctl is-active --quiet "$SERVICE_NAME"; then
  log "opspilot-agent is active"
else
  log "opspilot-agent is enabled; it will stay active once /etc/opspilot/agent.yaml is configured"
fi

cat <<'EOF'

Installation completed.

Next step:
1. Edit /etc/opspilot/agent.yaml

2. Restart:

sudo systemctl restart opspilot-agent
EOF
