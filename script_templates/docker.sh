# name: Docker Install
# path: /docker
# desc: Install Docker engine via official script
#!/bin/sh
set -eu

if command -v docker >/dev/null 2>&1; then
  echo "[env-hub] Docker already installed: $(docker --version)"
  exit 0
fi

echo "[env-hub] Installing Docker..."
curl -fsSL https://get.docker.com | sh

if [ -n "${SUDO_USER:-}" ] && [ "$(id -u)" = "0" ]; then
  usermod -aG docker "$SUDO_USER" 2>/dev/null || true
  echo "[env-hub] Added $SUDO_USER to docker group. Re-login to take effect."
fi

echo "[env-hub] Docker installed: $(docker --version)"
