# name: Basic Dev Tools
# path: /dev
# desc: Install common development tools (curl, git, vim, build essentials)
#!/bin/sh
set -eu

PKGS_DEB="curl git vim build-essential ca-certificates"
PKGS_APK="curl git vim build-base ca-certificates"
PKGS_RPM="curl git vim gcc make ca-certificates"

SUDO=""
if [ "$(id -u)" != "0" ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    echo "[env-hub] Error: not root and sudo not found." >&2
    exit 1
  fi
fi

if command -v apt-get >/dev/null 2>&1; then
  echo "[env-hub] Detected Debian/Ubuntu. Installing: $PKGS_DEB"
  $SUDO apt-get update -qq
  $SUDO apt-get install -y --no-install-recommends $PKGS_DEB
elif command -v apk >/dev/null 2>&1; then
  echo "[env-hub] Detected Alpine. Installing: $PKGS_APK"
  $SUDO apk add --no-cache $PKGS_APK
elif command -v dnf >/dev/null 2>&1; then
  echo "[env-hub] Detected Fedora/RHEL. Installing: $PKGS_RPM"
  $SUDO dnf install -y $PKGS_RPM
elif command -v yum >/dev/null 2>&1; then
  echo "[env-hub] Detected CentOS/RHEL. Installing: $PKGS_RPM"
  $SUDO yum install -y $PKGS_RPM
else
  echo "[env-hub] Error: unsupported package manager." >&2
  exit 1
fi

echo "[env-hub] Dev tools installed successfully."
