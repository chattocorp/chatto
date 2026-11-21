#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 --app NAME --region CODE --domain HOSTNAME" >&2
  exit 2
}

app=''
region=''
domain=''
while (( $# )); do
  (( $# >= 2 )) || usage
  case "$1" in
    --app) app=$2 ;;
    --region) region=$2 ;;
    --domain) domain=$2 ;;
    *) usage ;;
  esac
  shift 2
done

label='[a-z0-9]([a-z0-9-]*[a-z0-9])?'
[[ $app =~ ^$label$ && $region =~ ^[a-z0-9]{3}$ && $domain =~ ^($label\.)+$label$ ]] || usage
port=${LOCAL_PORT:-14000}
[[ $port =~ ^[1-9][0-9]{0,4}$ ]] || usage
(( port <= 65535 )) || usage
for tool in flyctl curl jq; do
  command -v "$tool" >/dev/null || { echo "Install $tool first." >&2; exit 1; }
done

template=$(cd "$(dirname "$0")" && pwd)/fly.toml
dir="$PWD/fly-$app"
config="$dir/fly.toml"
local_url="http://127.0.0.1:$port"

if [[ ! -f $config ]]; then
  mkdir -p "$dir"
  # Create by name. A collision must never deploy over someone else's app.
  flyctl apps create "$app"
  sed -e "s/APP_NAME/$app/g" -e "s/APP_REGION/$region/g" -e "s/APP_DOMAIN/$domain/g" \
    "$template" > "$config.new"
  mv "$config.new" "$config"
fi
for setting in "app = \"$app\"" "primary_region = \"$region\"" "  CHATTO_WEBSERVER_URL = \"https://$domain\""; do
  grep -Fxq "$setting" "$config" || { echo "Arguments differ from $config" >&2; exit 1; }
done

if [[ ! -f $dir/published ]]; then
  private="$dir/setup.toml"
  sed -e '/^\[http_service\]/,$d' \
    -e "s|https://$domain|$local_url|" \
    -e 's/CHATTO_CORE_SKIP_SETUP_WIZARD = "true"/CHATTO_CORE_SKIP_SETUP_WIZARD = "false"/' \
    "$config" > "$private"
  flyctl deploy --config "$private" --ha=false --no-public-ips --yes
  flyctl proxy "$port:4000" --app "$app" &
  proxy_pid=$!
  trap 'kill "$proxy_pid" 2>/dev/null || true; wait "$proxy_pid" 2>/dev/null || true' EXIT
  trap 'exit 1' INT TERM

  if ! curl -fsS --retry 12 --retry-connrefused --retry-delay 1 --max-time 3 \
    "$local_url/readyz" >/dev/null; then
    echo "Private server is not ready. Check flyctl logs --app $app and retry." >&2
    exit 1
  fi
  echo "Open $local_url/setup to create the first owner. Keep this terminal open."
  while kill -0 "$proxy_pid" 2>/dev/null; do
    # ProtoJSON omits false fields. Require a valid discovery response as well.
    if curl -fsS --max-time 5 "$local_url/.well-known/chatto" 2>/dev/null |
      jq -e '.profile.version and .login and ((.setupRequired // false) == false)' >/dev/null; then
      break
    fi
    sleep 2
  done
  kill -0 "$proxy_pid" || { echo "Setup tunnel closed. Run the same command to retry." >&2; exit 1; }
  kill "$proxy_pid"
  wait "$proxy_pid" || true
  trap - EXIT INT TERM

  # Fly only allocates IPs automatically on the first deploy, which was private.
  ips=$(flyctl ips list --app "$app" --json)
  if ! jq -e 'any(.[]; (.Type // .type) == "shared_v4")' <<< "$ips" >/dev/null; then
    flyctl ips allocate-v4 --shared --app "$app"
  fi
  if ! jq -e 'any(.[]; (.Type // .type) == "v6")' <<< "$ips" >/dev/null; then
    flyctl ips allocate-v6 --app "$app"
  fi
  flyctl deploy --config "$config" --ha=false --yes
  touch "$dir/published"
fi

if ! flyctl certs setup "$domain" --app "$app"; then
  flyctl certs add "$domain" --app "$app"
  flyctl certs setup "$domain" --app "$app"
fi
echo "Apply the DNS records above at your DNS provider."
if [[ -t 0 ]]; then
  read -r -p "Press Enter when the DNS records are in place. "
fi
flyctl certs check "$domain" --app "$app"
if ! curl -fsS --max-time 10 "https://$domain/readyz" | jq -e '.status == "ready"' >/dev/null; then
  echo "HTTPS is not ready. Run the same command after DNS has propagated." >&2
  exit 1
fi
echo "Chatto is ready at https://$domain"
