import { fileURLToPath } from "node:url";
import { assertMacOSMediaUsageDescriptions } from "./macos_privacy.ts";
import { macOSVersions } from "./version.ts";

if (import.meta.main) {
  await installLegalNotices();
}

if (import.meta.main && Deno.build.os === "darwin") {
  const bundle = fileURLToPath(
    new URL("../dist/Chatto Desktop.app", import.meta.url),
  );
  const updateMarker = `${bundle}/Contents/MacOS/laufey.dylib.update-ok`;
  const denoConfig = JSON.parse(
    await Deno.readTextFile(new URL("../deno.json", import.meta.url)),
  ) as { version?: unknown };
  if (typeof denoConfig.version !== "string") {
    throw new TypeError("deno.json must declare the desktop version.");
  }
  const versions = macOSVersions(denoConfig.version);

  await assertMacOSMediaUsageDescriptions(`${bundle}/Contents/Info.plist`);
  await run("plutil", [
    "-replace",
    "CFBundleShortVersionString",
    "-string",
    versions.shortVersion,
    `${bundle}/Contents/Info.plist`,
  ]);
  await run("plutil", [
    "-replace",
    "CFBundleVersion",
    "-string",
    versions.bundleVersion,
    `${bundle}/Contents/Info.plist`,
  ]);

  // Deno 2.9.4 adds a configured AppIcon.icns after its internal signing pass.
  // A previously launched build may also contain Laufey's runtime marker.
  // Return the output to its pre-launch state, then seal all bundled resources.
  await Deno.remove(updateMarker).catch((error) => {
    if (!(error instanceof Deno.errors.NotFound)) throw error;
  });
  await run("codesign", ["--force", "--deep", "--sign", "-", bundle]);
  await run("codesign", [
    "--verify",
    "--deep",
    "--strict",
    "--verbose=2",
    bundle,
  ]);
}

async function installLegalNotices(): Promise<void> {
  const resources = fileURLToPath(
    new URL(
      Deno.build.os === "darwin"
        ? "../dist/Chatto Desktop.app/Contents/Resources/"
        : Deno.build.os === "windows"
        ? "../dist/windows/"
        : "../dist/chatto-desktop/",
      import.meta.url,
    ),
  );
  const licenses = `${resources}/LICENSES`;
  await Deno.mkdir(licenses, { recursive: true });

  const repositoryRoot = fileURLToPath(new URL("../../../", import.meta.url));
  await Deno.copyFile(`${repositoryRoot}/NOTICE`, `${resources}/NOTICE`);
  for (
    const filename of [
      "Apache-2.0.txt",
      "BSD-3-Clause.txt",
      "MIT.txt",
    ]
  ) {
    await Deno.copyFile(
      `${repositoryRoot}/LICENSES/${filename}`,
      `${licenses}/${filename}`,
    );
  }

  const desktopLegal = fileURLToPath(new URL("../legal/", import.meta.url));
  for await (const entry of Deno.readDir(desktopLegal)) {
    if (!entry.isFile) continue;
    await Deno.copyFile(
      `${desktopLegal}/${entry.name}`,
      `${licenses}/${entry.name}`,
    );
  }
}

async function run(command: string, args: string[]): Promise<void> {
  const result = await new Deno.Command(command, {
    args,
    stdin: "null",
    stdout: "inherit",
    stderr: "inherit",
  }).output();
  if (!result.success) {
    throw new Error(`${command} exited with code ${result.code}`);
  }
}
