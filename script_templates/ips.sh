# name: Client IPs (JSON)
# path: /ips
# desc: Probe client IPv4 and IPv6 as JSON
#!/bin/sh
set -eu

BASE_URL="__BASE_URL__"

fetch_ip() {
  curl "$1" -fsSL --connect-timeout 3 --max-time 6 "$BASE_URL/ip" 2>/dev/null \
    | sed -n 's/.*"ip":"\([^"]*\)".*/\1/p'
}

ipv4="$(fetch_ip -4 || true)"
ipv6="$(fetch_ip -6 || true)"

printf '{"ipv4":"%s","ipv6":"%s"}\n' "$ipv4" "$ipv6"
