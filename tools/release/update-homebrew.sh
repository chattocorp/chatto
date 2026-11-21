#!/usr/bin/env bash
# Point the chattocorp/homebrew-tap formula at a published Chatto release.
# Run it only for the highest stable version, so a maintenance release of an
# older line does not move the formula back.
#
# Environment: VERSION, GH_TOKEN (reads the release), HOMEBREW_TAP_GITHUB_TOKEN
# (pushes to the tap).
set -euo pipefail

: "${VERSION:?}"
if [[ -z "${HOMEBREW_TAP_GITHUB_TOKEN:-}" ]]; then
  echo "::error::HOMEBREW_TAP_GITHUB_TOKEN is required to update chattocorp/homebrew-tap."
  exit 1
fi

workdir="$(mktemp -d)"
checksums="${workdir}/chatto_${VERSION}_checksums.txt"

gh release download "v${VERSION}" \
  --repo chattocorp/chatto \
  --pattern "chatto_${VERSION}_checksums.txt" \
  --dir "${workdir}"

checksum_for() {
  awk -v artifact="$1" '$2 == artifact { print $1 }' "${checksums}"
}

darwin_arm64_sha="$(checksum_for chatto_Darwin_arm64.tar.gz)"
linux_arm64_sha="$(checksum_for chatto_Linux_arm64.tar.gz)"
linux_x86_64_sha="$(checksum_for chatto_Linux_x86_64.tar.gz)"

for checksum in darwin_arm64_sha linux_arm64_sha linux_x86_64_sha; do
  if [[ -z "${!checksum}" ]]; then
    echo "::error::Missing ${checksum} in ${checksums}."
    exit 1
  fi
done

tap_dir="${workdir}/homebrew-tap"
git clone "https://x-access-token:${HOMEBREW_TAP_GITHUB_TOKEN}@github.com/chattocorp/homebrew-tap.git" "${tap_dir}"

mkdir -p "${tap_dir}/Formula"
cat > "${tap_dir}/Formula/chatto.rb" <<FORMULA
class Chatto < Formula
  desc "Real-time chat app CLI and server"
  homepage "https://github.com/chattocorp/chatto"
  version "${VERSION}"
  license "AGPL-3.0-or-later"

  on_macos do
    on_arm do
      url "https://github.com/chattocorp/chatto/releases/download/v#{version}/chatto_Darwin_arm64.tar.gz"
      sha256 "${darwin_arm64_sha}"
    end

    on_intel do
      odie "Chatto currently provides Homebrew builds for Apple Silicon macOS only."
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chattocorp/chatto/releases/download/v#{version}/chatto_Linux_arm64.tar.gz"
      sha256 "${linux_arm64_sha}"
    end

    on_intel do
      url "https://github.com/chattocorp/chatto/releases/download/v#{version}/chatto_Linux_x86_64.tar.gz"
      sha256 "${linux_x86_64_sha}"
    end
  end

  def install
    bin.install "chatto"
    system "codesign", "--force", "--sign", "-", bin/"chatto" if OS.mac?
  end

  test do
    assert_match "chatto version #{version}", shell_output("#{bin}/chatto version")
  end
end
FORMULA

git -C "${tap_dir}" config user.name "github-actions[bot]"
git -C "${tap_dir}" config user.email "41898282+github-actions[bot]@users.noreply.github.com"

if git -C "${tap_dir}" diff --quiet -- Formula/chatto.rb; then
  echo "Homebrew tap already contains Chatto ${VERSION}."
  exit 0
fi

git -C "${tap_dir}" add Formula/chatto.rb
git -C "${tap_dir}" commit -m "chore: update chatto to v${VERSION}"
git -C "${tap_dir}" push origin HEAD:main
