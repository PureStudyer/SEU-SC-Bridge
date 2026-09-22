#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
ARCH="${1:-arm64}"
VERSION="${VERSION:-1.0.1}"
case "$ARCH" in amd64|arm64) ;; *) echo 'architecture must be amd64 or arm64' >&2; exit 1 ;; esac
APP="$PWD/dist/macos-$ARCH/SEU SC Bridge.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" go build -trimpath -tags desktop,production -ldflags "-s -w -X github.com/PureStudyer/SEU-SC-Bridge/internal/agent.Version=$VERSION" -o "$APP/Contents/MacOS/seusc" ./cmd/seusc
cp packaging/macos/app-icon.icns "$APP/Contents/Resources/app-icon.icns"
cp packaging/macos/Info.plist "$APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$APP/Contents/Info.plist"
if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
 codesign --force --deep --options runtime --timestamp --entitlements packaging/macos/entitlements.plist --sign "$APPLE_SIGNING_IDENTITY" "$APP"
else
 codesign --force --deep --sign - "$APP"
fi
STAGE="$PWD/dist/macos-$ARCH/dmg-root"
mkdir -p "$STAGE"
ditto "$APP" "$STAGE/SEU SC Bridge.app"
ln -sfn /Applications "$STAGE/Applications"
hdiutil create -volname "SEU SC Bridge" -srcfolder "$STAGE" -ov -format UDZO "$PWD/dist/SEU-SC-Bridge-$ARCH.dmg"
if [[ -n "${APPLE_NOTARY_KEY_PATH:-}" ]]; then
 : "${APPLE_NOTARY_KEY_ID:?APPLE_NOTARY_KEY_ID is required with APPLE_NOTARY_KEY_PATH}"
 : "${APPLE_NOTARY_ISSUER:?APPLE_NOTARY_ISSUER is required with APPLE_NOTARY_KEY_PATH}"
 xcrun notarytool submit "$PWD/dist/SEU-SC-Bridge-$ARCH.dmg" --key "$APPLE_NOTARY_KEY_PATH" --key-id "$APPLE_NOTARY_KEY_ID" --issuer "$APPLE_NOTARY_ISSUER" --wait
 xcrun stapler staple "$PWD/dist/SEU-SC-Bridge-$ARCH.dmg"
elif [[ -n "${APPLE_NOTARY_PROFILE:-}" ]]; then
 xcrun notarytool submit "$PWD/dist/SEU-SC-Bridge-$ARCH.dmg" --keychain-profile "$APPLE_NOTARY_PROFILE" --wait
 xcrun stapler staple "$PWD/dist/SEU-SC-Bridge-$ARCH.dmg"
fi
