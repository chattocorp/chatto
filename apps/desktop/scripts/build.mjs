import { execFileSync } from "node:child_process";
import { mkdir, rename, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { packager } from "@electron/packager";
import packageJson from "../package.json" with { type: "json" };
import { macOSVersions, releaseBuildVersion } from "./version.mjs";

const desktopRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const repositoryRoot = path.resolve(desktopRoot, "../..");
const distRoot = path.join(desktopRoot, "dist");
const packagerOut = path.join(distRoot, ".packager");
const platform = process.platform;
const macVersions =
  platform === "darwin" ? macOSVersions(packageJson.version) : undefined;

await rm(distRoot, { recursive: true, force: true });
await mkdir(packagerOut, { recursive: true });

const [bundleRoot] = await packager({
  dir: desktopRoot,
  out: packagerOut,
  overwrite: true,
  asar: true,
  prune: false,
  name: packageJson.productName,
  executableName: platform === "darwin" ? undefined : "chatto-desktop",
  appVersion: macVersions?.shortVersion ?? packageJson.version,
  buildVersion:
    macVersions?.bundleVersion ?? releaseBuildVersion(packageJson.version),
  appBundleId: "run.chatto.desktop",
  icon:
    platform === "win32"
      ? path.join(desktopRoot, "icons/icon.ico")
      : platform === "darwin"
        ? path.join(desktopRoot, "icons/icon.icns")
        : undefined,
  extraResource: [
    path.resolve(desktopRoot, "../frontend/build"),
    path.join(desktopRoot, "node_modules/electron/dist/LICENSE"),
    path.join(desktopRoot, "node_modules/electron/dist/LICENSES.chromium.html"),
    path.join(repositoryRoot, "NOTICE"),
    path.join(repositoryRoot, "LICENSES"),
  ],
  usageDescription: {
    Camera: "Chatto uses the camera when you choose to share video in a call.",
    Microphone:
      "Chatto uses the microphone when you join a voice or video call.",
  },
  ignore: [
    /^\/dist(?:\/|$)/,
    /^\/node_modules(?:\/|$)/,
    /^\/scripts(?:\/|$)/,
    /\.test\.mjs$/,
  ],
});

if (platform === "darwin") {
  const appBundle = path.join(bundleRoot, `${packageJson.productName}.app`);
  execFileSync("codesign", ["--force", "--deep", "--sign", "-", appBundle]);
  await rename(
    appBundle,
    path.join(distRoot, `${packageJson.productName}.app`),
  );
} else if (platform === "win32") {
  await rename(bundleRoot, path.join(distRoot, "windows"));
} else {
  await rename(bundleRoot, path.join(distRoot, "chatto-desktop"));
}

await rm(packagerOut, { recursive: true, force: true });
