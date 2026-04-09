# name: Node.js (via nvm)
# path: /node
# desc: Install Node.js via nvm (Node Version Manager)
#!/bin/sh
set -eu

NVM_VERSION="${NVM_VERSION:-v0.40.1}"
NODE_VERSION="${NODE_VERSION:-lts/*}"

if [ -d "$HOME/.nvm" ]; then
  echo "[env-hub] nvm already installed at $HOME/.nvm"
else
  echo "[env-hub] Installing nvm $NVM_VERSION..."
  curl -fsSL "https://raw.githubusercontent.com/nvm-sh/nvm/$NVM_VERSION/install.sh" | sh
fi

# Load nvm into current shell
export NVM_DIR="$HOME/.nvm"
# shellcheck disable=SC1091
[ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"

if command -v nvm >/dev/null 2>&1; then
  echo "[env-hub] Installing Node.js ($NODE_VERSION)..."
  nvm install "$NODE_VERSION"
  nvm use "$NODE_VERSION"
  echo "[env-hub] Done: node $(node --version), npm $(npm --version)"
else
  echo "[env-hub] nvm installed. Restart your shell and run: nvm install $NODE_VERSION"
fi
