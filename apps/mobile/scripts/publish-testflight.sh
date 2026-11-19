#!/usr/bin/env bash
# Sign a device archive and upload it using an App Store Connect team API key.
# Run only on an ephemeral macOS CI runner, after mobile-ios-sync.
set -euo pipefail
umask 077

for variable in IOS_CERTIFICATE_BASE64 IOS_CERTIFICATE_PASSWORD IOS_PROFILE_BASE64 \
  ASC_API_KEY_BASE64 ASC_API_KEY_ID ASC_API_ISSUER_ID \
  IOS_MARKETING_VERSION IOS_BUILD_NUMBER RUNNER_TEMP; do
  if [[ -z "${!variable:-}" ]]; then
    echo "::error::Required setting ${variable} is missing."
    exit 1
  fi
done

work="$(mktemp -d "$RUNNER_TEMP/chatto-testflight.XXXXXX")"
keychain="$work/signing.keychain-db"
profile_path=''
# The runner is disposable. Remove credentials on success, failure, or signal.
cleanup() {
  security delete-keychain "$keychain" >/dev/null 2>&1 || true
  if [[ -n "$profile_path" ]]; then rm -f "$profile_path"; fi
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

printf '%s' "$IOS_CERTIFICATE_BASE64" | base64 --decode > "$work/certificate.p12"
printf '%s' "$IOS_PROFILE_BASE64" | base64 --decode > "$work/profile.mobileprovision"
printf '%s' "$ASC_API_KEY_BASE64" | base64 --decode > "$work/AuthKey.p8"
security cms -D -i "$work/profile.mobileprovision" > "$work/profile.plist"
profile_uuid="$(/usr/libexec/PlistBuddy -c 'Print UUID' "$work/profile.plist")"
team_id="$(/usr/libexec/PlistBuddy -c 'Print TeamIdentifier:0' "$work/profile.plist")"
app_id="$(/usr/libexec/PlistBuddy -c 'Print Entitlements:application-identifier' "$work/profile.plist")"
if [[ ! "$profile_uuid" =~ ^[0-9A-Fa-f-]+$ || ! "$team_id" =~ ^[A-Z0-9]+$ ||
      "$app_id" != "$team_id.eu.chattocorp.chatto.mobile" ]]; then
  echo '::error::The provisioning profile does not match Chatto Mobile.'
  exit 1
fi
profile_directory="$HOME/Library/Developer/Xcode/UserData/Provisioning Profiles"
mkdir -p "$profile_directory"
profile_path="$profile_directory/$profile_uuid.mobileprovision"
cp "$work/profile.mobileprovision" "$profile_path"

keychain_password="$(openssl rand -hex 24)"
security create-keychain -p "$keychain_password" "$keychain"
security set-keychain-settings -lut 3600 "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
security import "$work/certificate.p12" -k "$keychain" \
  -P "$IOS_CERTIFICATE_PASSWORD" -T /usr/bin/codesign -T /usr/bin/security >/dev/null
security set-key-partition-list -S apple-tool:,apple: -s -k "$keychain_password" "$keychain" >/dev/null
security list-keychains -d user -s "$keychain"

xcodebuild -quiet -project apps/mobile/ios/App/App.xcodeproj -scheme Chatto \
  -configuration Release -destination 'generic/platform=iOS' \
  -archivePath "$work/Chatto.xcarchive" \
  -derivedDataPath "$work/DerivedData" \
  "MARKETING_VERSION=$IOS_MARKETING_VERSION" "CURRENT_PROJECT_VERSION=$IOS_BUILD_NUMBER" \
  "DEVELOPMENT_TEAM=$team_id" CODE_SIGN_STYLE=Manual \
  'CODE_SIGN_IDENTITY=Apple Distribution' "PROVISIONING_PROFILE_SPECIFIER=$profile_uuid" archive

# Disable Xcode's automatic renumbering so the uploaded build matches the summary.
cat > "$work/ExportOptions.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>method</key><string>app-store-connect</string>
  <key>destination</key><string>upload</string>
  <key>teamID</key><string>$team_id</string>
  <key>signingStyle</key><string>manual</string>
  <key>signingCertificate</key><string>Apple Distribution</string>
  <key>provisioningProfiles</key><dict>
    <key>eu.chattocorp.chatto.mobile</key><string>$profile_uuid</string>
  </dict>
  <key>manageAppVersionAndBuildNumber</key><false/>
  <key>uploadSymbols</key><true/>
</dict></plist>
PLIST
xcodebuild -exportArchive -archivePath "$work/Chatto.xcarchive" \
  -exportOptionsPlist "$work/ExportOptions.plist" -exportPath "$work/export" \
  -authenticationKeyPath "$work/AuthKey.p8" \
  -authenticationKeyID "$ASC_API_KEY_ID" -authenticationKeyIssuerID "$ASC_API_ISSUER_ID"
printf '%s\n' '- Upload accepted. Check App Store Connect for processing and tester availability.' >> "$GITHUB_STEP_SUMMARY"
