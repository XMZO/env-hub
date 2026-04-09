# name: SSH Key Install
# path: /ssh
# desc: Install SSH public keys from this server
#!/bin/sh
# env-hub: install SSH public keys
set -eu

# Colors (disabled if not a tty or NO_COLOR is set)
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_GREEN='\033[32m'; C_YELLOW='\033[33m'; C_RED='\033[31m'; C_DIM='\033[2m'; C_OFF='\033[0m'
else
  C_GREEN=''; C_YELLOW=''; C_RED=''; C_DIM=''; C_OFF=''
fi

log()  { printf '%b[env-hub]%b %s\n' "$C_DIM" "$C_OFF" "$1"; }
ok()   { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_GREEN" "$1" "$C_OFF"; }
warn() { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_YELLOW" "$1" "$C_OFF"; }
err()  { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_RED" "$1" "$C_OFF" >&2; }

# Check dependencies
command -v curl >/dev/null 2>&1 || { err "curl is required but not installed."; exit 1; }

# Determine target SSH directory (respect SUDO_USER)
if [ -n "${SUDO_USER:-}" ] && [ "$(id -u)" = "0" ]; then
  SSH_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
  OWNER="$SUDO_USER"
else
  SSH_HOME="$HOME"
  OWNER=""
fi
SSH_DIR="$SSH_HOME/.ssh"
AUTH="$SSH_DIR/authorized_keys"

# Fetch keys
BASE_URL="${ENV_HUB_URL:-__BASE_URL__}"
log "Fetching keys from $BASE_URL/keys/main.pub"
KEYS=$(curl -fsSL "$BASE_URL/keys/main.pub") || { err "Failed to fetch keys."; exit 1; }
[ -z "$KEYS" ] && { err "No keys found at remote."; exit 1; }

# Prepare directory
mkdir -p "$SSH_DIR"
chmod 700 "$SSH_DIR"
touch "$AUTH"
chmod 600 "$AUTH"

# Describe a key for logging: "algo ...last8 comment"
describe_key() {
  # $1 = full key line; extract algo, fingerprint tail, comment
  algo=$(printf '%s' "$1" | awk '{print $1}')
  blob=$(printf '%s' "$1" | awk '{print $2}')
  comment=$(printf '%s' "$1" | cut -d' ' -f3-)
  tail=$(printf '%s' "$blob" | tail -c 12)
  if [ -n "$comment" ]; then
    printf '%s ...%s %s' "$algo" "$tail" "$comment"
  else
    printf '%s ...%s' "$algo" "$tail"
  fi
}

# Install keys line by line
ADDED=0
SKIPPED=0
TOTAL=0
OLDIFS=$IFS
IFS='
'
for KEY in $KEYS; do
  [ -z "$KEY" ] && continue
  TOTAL=$((TOTAL + 1))
  DESC=$(describe_key "$KEY")
  if grep -qxF "$KEY" "$AUTH" 2>/dev/null; then
    SKIPPED=$((SKIPPED + 1))
    log "skipped: $DESC"
  else
    printf '%s\n' "$KEY" >> "$AUTH"
    ADDED=$((ADDED + 1))
    ok "added:   $DESC"
  fi
done
IFS=$OLDIFS

# Fix ownership if running via sudo
if [ -n "$OWNER" ]; then
  chown -R "$OWNER:$OWNER" "$SSH_DIR"
fi

# Summary
if [ "$ADDED" -gt 0 ]; then
  ok "Added $ADDED key(s), skipped $SKIPPED (already present). Total: $TOTAL."
else
  warn "All $TOTAL key(s) already present, nothing to do."
fi
