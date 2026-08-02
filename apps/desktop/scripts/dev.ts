import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import denoConfig from "../deno.json" with { type: "json" };
import { assertMacOSMediaUsageDescriptions } from "./macos_privacy.ts";

const desktopRoot = fileURLToPath(new URL("../", import.meta.url));
const environment: Record<string, string> = {};

if (Deno.build.os === "darwin") {
  const appBundle = join(
    desktopRoot,
    "dist",
    `${denoConfig.desktop.app.name}.app`,
  );
  await assertMacOSMediaUsageDescriptions(
    join(appBundle, "Contents", "Info.plist"),
  );

  // Deno's generic cached --hmr app has no TCC usage descriptions, so macOS
  // terminates it as soon as CEF requests a microphone or camera. Clone the
  // locally packaged app as the Laufey development host instead. Deno re-signs
  // HMR hosts in place, so this must not be a link to the build output.
  const laufeyDevRoot = join(desktopRoot, "dist", "dev-laufey");
  const hostLink = join(
    laufeyDevRoot,
    "cef",
    "build",
    "Release",
    "laufey.app",
  );
  await Deno.mkdir(dirname(hostLink), { recursive: true });
  try {
    const currentHost = await Deno.lstat(hostLink);
    if (!currentHost.isDirectory && !currentHost.isSymlink) {
      throw new Error(`Refusing to replace unexpected HMR host: ${hostLink}`);
    }
    await Deno.remove(hostLink, { recursive: currentHost.isDirectory });
  } catch (error) {
    if (!(error instanceof Deno.errors.NotFound)) throw error;
  }
  await run("/bin/cp", [
    // APFS clone-on-write keeps the private 300 MB CEF host cheap.
    "-cR",
    appBundle,
    hostLink,
  ]);

  // The packaged runtime dylib has its own generated identifier. Deno's HMR
  // launcher would change it after resolving the host and thereby invalidate
  // the outer seal. Harmonize it first, then re-seal without --deep so nested
  // identifiers and helper signatures are preserved.
  const runtime = join(hostLink, "Contents", "MacOS", "laufey.dylib");
  await run("/usr/bin/codesign", [
    "--force",
    "--sign",
    "-",
    "--identifier",
    denoConfig.desktop.app.identifier,
    runtime,
  ]);
  await run("/usr/bin/codesign", ["--force", "--sign", "-", hostLink]);
  await run("/usr/bin/codesign", [
    "--verify",
    "--deep",
    "--strict",
    hostLink,
  ]);
  environment.LAUFEY_DEV_DIR = laufeyDevRoot;
}

const command = new Deno.Command(Deno.execPath(), {
  args: [
    "desktop",
    "--hmr",
    "--allow-net=127.0.0.1,localhost",
    "--allow-read=../frontend/build",
    "--include=../frontend/build",
    "main.ts",
  ],
  cwd: desktopRoot,
  env: environment,
  stdin: "inherit",
  stdout: "inherit",
  stderr: "inherit",
});
const status = await command.spawn().status;
if (!status.success) Deno.exit(status.code);

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
