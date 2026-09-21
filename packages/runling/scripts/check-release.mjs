import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

export function assertUnusedVersion(version, metadata) {
  if (
    Object.hasOwn(metadata.versions ?? {}, version) ||
    Object.hasOwn(metadata.time ?? {}, version)
  ) {
    throw new Error(
      `runling@${version} has already been published, possibly later unpublished. Choose a new version.`,
    );
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const { version } = JSON.parse(
    await readFile(
      new URL("../package.json", import.meta.url),
      "utf8",
    ),
  );
  const response = await fetch("https://registry.npmjs.org/runling", {
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok)
    throw new Error(
      `Cannot verify npm version history: HTTP ${response.status}`,
    );
  assertUnusedVersion(version, await response.json());
  console.log(
    `runling@${version} is absent from npm's published versions and historical timestamps.`,
  );
}
