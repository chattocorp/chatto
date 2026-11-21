#!/usr/bin/env bash
# Import the Developer ID certificate and notary key into a temporary keychain,
# and export the paths and identity that `mise desktop-build` reads.
#
# Environment: CERTIFICATE_BASE64, CERTIFICATE_PASSWORD, NOTARY_API_KEY_BASE64,
# NOTARY_API_KEY_ID, NOTARY_API_ISSUER_ID, RUNNER_TEMP, GITHUB_ENV.
set -euo pipefail

for variable in \
  CERTIFICATE_BASE64 \
  CERTIFICATE_PASSWORD \
  NOTARY_API_KEY_BASE64 \
  NOTARY_API_KEY_ID \
  NOTARY_API_ISSUER_ID; do
  if [[ -z "${!variable:-}" ]]; then
    echo "::error::The ${variable} macOS release secret is not configured."
    exit 1
  fi
done

certificate_path="$RUNNER_TEMP/chatto-developer-id.p12"
api_key_path="$RUNNER_TEMP/AuthKey_${NOTARY_API_KEY_ID}.p8"
keychain_path="$RUNNER_TEMP/chatto-signing.keychain-db"
keychain_password="$(openssl rand -hex 24)"

# Record the paths first so the cleanup step finds them after a failure.
{
  echo "CHATTO_MACOS_NOTARY_API_KEY=${api_key_path}"
  echo "CHATTO_MACOS_SIGNING_KEYCHAIN=${keychain_path}"
  echo "CHATTO_MACOS_SIGNING_CERTIFICATE=${certificate_path}"
} >> "$GITHUB_ENV"

printf '%s' "$CERTIFICATE_BASE64" | base64 --decode > "$certificate_path"
printf '%s' "$NOTARY_API_KEY_BASE64" | base64 --decode > "$api_key_path"
chmod 600 "$certificate_path" "$api_key_path"

security create-keychain -p "$keychain_password" "$keychain_path"
security set-keychain-settings -lut 21600 "$keychain_path"
security unlock-keychain -p "$keychain_password" "$keychain_path"
security import "$certificate_path" \
  -k "$keychain_path" \
  -P "$CERTIFICATE_PASSWORD" \
  -T /usr/bin/codesign \
  -T /usr/bin/security
security set-key-partition-list \
  -S apple-tool:,apple: \
  -s \
  -k "$keychain_password" \
  "$keychain_path"
security list-keychains -d user -s "$keychain_path"

signing_identity="$(
  security find-identity -v -p codesigning "$keychain_path" |
    awk '/Developer ID Application:/ { print $2; exit }'
)"
if [[ -z "$signing_identity" ]]; then
  echo "::error::The certificate does not contain a Developer ID Application identity."
  exit 1
fi

{
  echo "CHATTO_MACOS_SIGN_IDENTITY=${signing_identity}"
  echo "CHATTO_MACOS_NOTARY_API_KEY_ID=${NOTARY_API_KEY_ID}"
  echo "CHATTO_MACOS_NOTARY_API_ISSUER_ID=${NOTARY_API_ISSUER_ID}"
} >> "$GITHUB_ENV"
