import { mkdir, readdir, rename, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { packager } from "@electron/packager";
import packageJson from "../package.json" with { type: "json" };
import { pruneElectronLocales } from "./locales.mjs";
import { embedMacOSCaptureHelper } from "./macos-capture-helper.mjs";
import { macOSDistributionOptions } from "./macos-signing.mjs";
import { macOSVersions, releaseBuildVersion } from "./version.mjs";

const desktopRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const repositoryRoot = path.resolve(desktopRoot, "../..");
const distRoot = path.join(desktopRoot, "dist");
const packagerOut = path.join(distRoot, ".packager");
const platform = process.platform;
const electronChecksum = process.env.CHATTO_ELECTRON_CHECKSUM;
const electronArchiveName = `electron-v${packageJson.devDependencies.electron}-${platform}-${process.arch}.zip`;
const embedCaptureHelper = platform === "darwin";
const macVersions =
  platform === "darwin" ? macOSVersions(packageJson.version) : undefined;
const macOSDistribution = macOSDistributionOptions(platform, process.env);
const supportedLocales = (
  await readdir(path.resolve(desktopRoot, "../frontend/messages"), {
    withFileTypes: true,
  })
)
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name);

await rm(distRoot, { recursive: true, force: true });
await mkdir(packagerOut, { recursive: true });

const [bundleRoot] = await packager({
  dir: desktopRoot,
  out: packagerOut,
  overwrite: true,
  asar: true,
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
    AudioCapture:
      "Chatto captures game audio only while you choose to stream it.",
    Camera: "Chatto uses the camera when you choose to share video in a call.",
    Microphone:
      "Chatto uses the microphone when you join a voice or video call.",
  },
  download: electronChecksum
    ? { checksums: { [electronArchiveName]: electronChecksum } }
    : undefined,
  afterPrune: embedCaptureHelper
    ? [
        async ({ buildPath }) => {
          const appBundle = path.resolve(buildPath, "../../..");
          const prunedLocales = await pruneElectronLocales(
            appBundle,
            platform,
            supportedLocales,
          );
          logPrunedLocales(prunedLocales);
          await embedMacOSCaptureHelper(appBundle, macVersions);
        },
      ]
    : undefined,
  osxSign: macOSDistribution.osxSign,
  osxNotarize: macOSDistribution.osxNotarize,
  ignore: [
    /^\/dist(?:\/|$)/,
    /^\/native(?:\/|$)/,
    /^\/node_modules(?:\/|$)/,
    /^\/scripts(?:\/|$)/,
    /\.test\.mjs$/,
  ],
});

const appBundle =
  platform === "darwin"
    ? path.join(bundleRoot, `${packageJson.productName}.app`)
    : bundleRoot;
if (platform !== "darwin") {
  logPrunedLocales(
    await pruneElectronLocales(appBundle, platform, supportedLocales),
  );
}

if (platform === "darwin") {
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

function logPrunedLocales(prunedLocales) {
  console.log(
    `Removed ${prunedLocales.removedLocales} unused Electron locale resources (${(prunedLocales.removedBytes / 1024 / 1024).toFixed(1)} MiB)`,
  );
}
