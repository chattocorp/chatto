// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { execFileSync } from "node:child_process";
import { copyFile, mkdir, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const windowsCaptureHelperExecutable =
  "chatto-windows-capture-probe.exe";

const scriptsRoot = path.dirname(fileURLToPath(import.meta.url));
const probeRoot = path.resolve(scriptsRoot, "../native/windows-capture-probe");
const visualCppRuntimeFiles = [
  "msvcp140.dll",
  "msvcp140_atomic_wait.dll",
  "vcruntime140.dll",
  "vcruntime140_1.dll",
];

/** Build and embed the pinned Windows capture helper and its LiveKit runtime. */
export async function embedWindowsCaptureHelper(resourcesDirectory) {
  if (process.platform !== "win32" || process.arch !== "x64") {
    throw new Error("The Windows capture helper requires Windows x64.");
  }
  const buildDirectory = path.join(probeRoot, "build-package");
  const cmake = findCmakeExecutable();
  execFileSync(
    cmake,
    ["-S", probeRoot, "-B", buildDirectory, "-A", "x64"],
    { stdio: "inherit" },
  );
  execFileSync(
    cmake,
    [
      "--build",
      buildDirectory,
      "--config",
      "Release",
      "--target",
      windowsCaptureHelperExecutable.replace(/\.exe$/, ""),
    ],
    { stdio: "inherit" },
  );

  const destination = path.join(resourcesDirectory, "windows-capture");
  await rm(destination, { recursive: true, force: true });
  await mkdir(destination, { recursive: true });
  for (const file of [
    windowsCaptureHelperExecutable,
    "livekit.dll",
    "livekit_ffi.dll",
  ]) {
    await copyFile(
      path.join(buildDirectory, "Release", file),
      path.join(destination, file),
    );
  }
  const runtimeDirectory = findVisualCppRuntimeDirectory();
  for (const file of visualCppRuntimeFiles) {
    await copyFile(
      path.join(runtimeDirectory, file),
      path.join(destination, file),
    );
  }
  return destination;
}

function findCmakeExecutable() {
  if (process.env.CMAKE) {
    return process.env.CMAKE;
  }
  try {
    const matches = execFileSync("where.exe", ["cmake"], {
      encoding: "utf8",
      windowsHide: true,
    })
      .split(/\r?\n/)
      .filter(Boolean);
    if (matches.length > 0) {
      return matches[0];
    }
  } catch {
    // Fall through to the CMake bundled with Visual Studio Build Tools.
  }
  return path.join(
    findVisualStudioInstallation(),
    "Common7",
    "IDE",
    "CommonExtensions",
    "Microsoft",
    "CMake",
    "CMake",
    "bin",
    "cmake.exe",
  );
}

function findVisualCppRuntimeDirectory() {
  const vswhere = findVisualStudioInstaller();
  const matches = execFileSync(
    vswhere,
    [
      "-latest",
      "-products",
      "*",
      "-find",
      "VC\\Redist\\MSVC\\**\\x64\\Microsoft.VC143.CRT\\msvcp140.dll",
    ],
    { encoding: "utf8" },
  )
    .split(/\r?\n/)
    .filter((entry) => entry && !entry.toLowerCase().includes("\\onecore\\"));
  if (matches.length === 0) {
    throw new Error("The Visual C++ x64 redistributable files are unavailable.");
  }
  return path.dirname(matches[0]);
}

function findVisualStudioInstallation() {
  const installation = execFileSync(
    findVisualStudioInstaller(),
    [
      "-latest",
      "-products",
      "*",
      "-requires",
      "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
      "-property",
      "installationPath",
    ],
    { encoding: "utf8" },
  ).trim();
  if (!installation) {
    throw new Error("A Visual Studio installation with C++ tools is required.");
  }
  return installation;
}

function findVisualStudioInstaller() {
  const programFilesX86 = process.env["ProgramFiles(x86)"];
  if (!programFilesX86) {
    throw new Error("The Visual Studio installer directory is unavailable.");
  }
  return path.join(
    programFilesX86,
    "Microsoft Visual Studio",
    "Installer",
    "vswhere.exe",
  );
}
