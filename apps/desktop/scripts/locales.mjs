import { readdir, rm, stat } from "node:fs/promises";
import path from "node:path";

const macOSGenderSuffix = /_(?:FEMININE|MASCULINE|NEUTER)$/;

function normaliseElectronLocale(entryName) {
  return entryName
    .replace(/\.(?:lproj|pak)$/, "")
    .replace(macOSGenderSuffix, "")
    .replaceAll("_", "-");
}

/**
 * Returns whether an Electron locale resource can serve one of the frontend's
 * locales. Electron uses both language-only names and platform-specific tag
 * formats, so a base pack such as `de` serves every supported German variant.
 */
export function isElectronLocaleSupported(entryName, supportedLocales) {
  const locale = normaliseElectronLocale(entryName);
  const language = locale.split("-", 1)[0];

  return supportedLocales.some(
    (supportedLocale) =>
      supportedLocale === locale || supportedLocale.startsWith(`${language}-`),
  );
}

async function entrySize(entryPath) {
  const entry = await stat(entryPath);
  if (!entry.isDirectory()) {
    return entry.size;
  }

  const children = await readdir(entryPath);
  const sizes = await Promise.all(
    children.map((child) => entrySize(path.join(entryPath, child))),
  );
  return sizes.reduce((total, size) => total + size, 0);
}

async function pruneLocaleDirectory(directory, extension, supportedLocales) {
  let entries;
  try {
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    if (error.code === "ENOENT") {
      return { removedBytes: 0, removedLocales: 0 };
    }
    throw error;
  }

  const removable = entries.filter(
    (entry) =>
      entry.name.endsWith(extension) &&
      !isElectronLocaleSupported(entry.name, supportedLocales),
  );
  const sizes = await Promise.all(
    removable.map((entry) => entrySize(path.join(directory, entry.name))),
  );
  await Promise.all(
    removable.map((entry) =>
      rm(path.join(directory, entry.name), { recursive: true, force: true }),
    ),
  );

  return {
    removedBytes: sizes.reduce((total, size) => total + size, 0),
    removedLocales: removable.length,
  };
}

/** Removes packaged native locale resources which the Chatto UI cannot use. */
export async function pruneElectronLocales(
  bundleRoot,
  platform,
  supportedLocales,
) {
  const locations =
    platform === "darwin"
      ? [
          path.join(bundleRoot, "Contents", "Resources"),
          path.join(
            bundleRoot,
            "Contents",
            "Frameworks",
            "Electron Framework.framework",
            "Versions",
            "A",
            "Resources",
          ),
        ].map((directory) => [directory, ".lproj"])
      : [[path.join(bundleRoot, "locales"), ".pak"]];

  const results = await Promise.all(
    locations.map(([directory, extension]) =>
      pruneLocaleDirectory(directory, extension, supportedLocales),
    ),
  );

  return results.reduce(
    (total, result) => ({
      removedBytes: total.removedBytes + result.removedBytes,
      removedLocales: total.removedLocales + result.removedLocales,
    }),
    { removedBytes: 0, removedLocales: 0 },
  );
}
