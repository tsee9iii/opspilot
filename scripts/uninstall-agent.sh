#!/usr/bin/env bash
#
# OpsPilot Agent uninstaller
#
# Removes the opspilot-agent systemd service and binary from a Linux host. It
# first attempts to unregister the agent with the Central server using the
# credentials in /etc/opspilot/agent.yaml.
#
# The script never removes logs. Configuration (/etc/opspilot/) is removed only
# after explicit confirmation.
#
# Usage: sudo scripts/uninstall-agent.sh

set -euo pipefail

readonly SERVICE_NAME="opspilot-agent"
readonly BIN_PATH="/usr/local/bin/opspilot-agent"
readonly SERVICE_PATH="/etc/systemd/system/opspilot-agent.service"
readonly CONFIG_PATH="/etc/opspilot/agent.yaml"

log() { printf '[uninstaller] %s\n' "$*"; }
die() { printf '[uninstaller] error: %s\n' "$*" >&2; exit 1; }

confirm() {
  # confirm PROMPT -> asks Y/n, defaults to n.
  local prompt="$1" answer
  printf '[uninstaller] %s (Y/n): ' "$prompt"
  read -r answer
  case "${answer:-n}" in
    y|Y|yes) return 0 ;;
    *) return 1 ;;
  esac
}

yaml_value() {
  # yaml_value KEY -> the value of a top-level `key: value` line, quotes stripped.
  sed -n "s/^${1}[[:space:]]*:[[:space:]]*\"\?\([^\"]*\)\"\?[[:space:]]*$/\1/p" "$CONFIG_PATH" | head -n 1
}

json_quote() {
  # json_quote STRING -> a JSON string literal (escapes backslash and quote).
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/^/"/; s/$/"/'
}

[[ "$(id -u)" -eq 0 ]] || die "run as root (sudo)"

command -v systemctl >/dev/null 2>&1 || die "systemd (systemctl) is required"

# --- 1. stop the agent ---------------------------------------------------------

if [[ -f "$SERVICE_PATH" ]]; then
  log "stopping ${SERVICE_NAME}"
  systemctl stop "$SERVICE_NAME" || log "warning: could not stop ${SERVICE_NAME} (is it running?)"
else
  log "no systemd unit found; nothing to stop"
fi

# --- 2. attempt unregister ------------------------------------------------------

central_url="$(yaml_value central_url)"
agent_id="$(yaml_value agent_id)"
secret="$(yaml_value secret)"

unregister_ok=false
if [[ -n "$central_url" && -n "$agent_id" && -n "$secret" ]]; then
  url="${central_url%/}/api/v1/agents/unregister"
  log "attempting unregister: POST ${url}"
  body=$(printf '{"agent_id":%s,"secret":%s}' "$(json_quote "$agent_id")" "$(json_quote "$secret")")
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS -o /dev/null -X POST "$url" \
      -H 'Content-Type: application/json' -d "$body"; then
      log "agent unregistered"
      unregister_ok=true
    fi
  fi
fi

if [[ "$unregister_ok" != true ]]; then
  log "warning: could not unregister the agent (missing config, missing curl, or central rejected the request)"
  if ! confirm "Continue uninstall?"; then
    log "uninstall aborted"
    exit 1
  fi
fi

# --- 5. disable the service ------------------------------------------------------

if [[ -f "$SERVICE_PATH" ]]; then
  log "disabling ${SERVICE_NAME}"
  systemctl disable "$SERVICE_NAME" 2>/dev/null || log "warning: could not disable ${SERVICE_NAME}"

  # --- 6. remove the systemd service -----------------------------------------------
  log "removing ${SERVICE_PATH}"
  rm -f "$SERVICE_PATH"
  systemctl daemon-reload 2>/dev/null || true
else
  log "no systemd unit found; nothing to disable"
fi

# --- 7. remove the binary -----------------------------------------------------------

if [[ -f "$BIN_PATH" ]]; then
  log "removing ${BIN_PATH}"
  rm -f "$BIN_PATH"
else
  log "no binary found at ${BIN_PATH}"
fi

# --- 8. remove configuration (optional) ---------------------------------------------

if [[ -d "/etc/opspilot" ]]; then
  if confirm "Remove configuration?"; then
    log "removing /etc/opspilot/"
    rm -rf /etc/opspilot
  else
    log "keeping /etc/opspilot/"
  fi
fi

log "uninstall complete"
