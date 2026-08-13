// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { execFileSync } from "node:child_process";
import { chmod, copyFile, cp, mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const macOSCaptureHelperAppName = "Chatto Capture Helper.app";
export const macOSCaptureHelperExecutable = "chatto-macos-capture-probe";

const scriptsRoot = path.dirname(fileURLToPath(import.meta.url));
const probeRoot = path.resolve(scriptsRoot, "../native/macos-capture-probe");

/**
 * Builds the ScreenCaptureKit probe and embeds it as a nested background app.
 * The surrounding Electron packager signs this bundle with the rest of Chatto.
 */
export async function embedMacOSCaptureHelper(
  appBundle,
  { shortVersion, bundleVersion },
) {
  if (process.platform !== "darwin") {
    throw new Error("The macOS capture helper can only be built on macOS.");
  }

  execFileSync(
    "xcrun",
    ["swift", "build", "--package-path", probeRoot, "-c", "release"],
    { stdio: "inherit" },
  );
  const swiftBinPath = execFileSync(
    "xcrun",
    [
      "swift",
      "build",
      "--package-path",
      probeRoot,
      "-c",
      "release",
      "--show-bin-path",
    ],
    { encoding: "utf8" },
  ).trim();

  const helperBundle = path.join(
    appBundle,
    "Contents",
    "Helpers",
    macOSCaptureHelperAppName,
  );
  const contents = path.join(helperBundle, "Contents");
  const executableDirectory = path.join(contents, "MacOS");
  const frameworksDirectory = path.join(contents, "Frameworks");
  const executable = path.join(
    executableDirectory,
    macOSCaptureHelperExecutable,
  );

  await rm(helperBundle, { recursive: true, force: true });
  await mkdir(executableDirectory, { recursive: true });
  await mkdir(frameworksDirectory, { recursive: true });
  await copyFile(
    path.join(swiftBinPath, macOSCaptureHelperExecutable),
    executable,
  );
  await chmod(executable, 0o755);
  for (const framework of [
    "LiveKitWebRTC.framework",
    "RustLiveKitUniFFI.framework",
  ]) {
    await cp(
      path.join(swiftBinPath, framework),
      path.join(frameworksDirectory, framework),
      {
        recursive: true,
        verbatimSymlinks: true,
      },
    );
  }
  execFileSync(
    "xcrun",
    [
      "install_name_tool",
      "-add_rpath",
      "@executable_path/../Frameworks",
      executable,
    ],
    { stdio: "inherit" },
  );
  await writeFile(
    path.join(contents, "Info.plist"),
    helperInfoPlist({ shortVersion, bundleVersion }),
  );

  return helperBundle;
}

function helperInfoPlist({ shortVersion, bundleVersion }) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>Chatto Capture Helper</string>
  <key>CFBundleExecutable</key>
  <string>${macOSCaptureHelperExecutable}</string>
  <key>CFBundleIdentifier</key>
  <string>run.chatto.desktop.capture-helper</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>Chatto Capture Helper</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${shortVersion}</string>
  <key>CFBundleVersion</key>
  <string>${bundleVersion}</string>
  <key>LSMinimumSystemVersion</key>
  <string>15.0</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSAudioCaptureUsageDescription</key>
  <string>Chatto captures game audio only while you choose to stream it.</string>
</dict>
</plist>
`;
}
