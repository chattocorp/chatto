import assert from "node:assert/strict";
import { mkdtemp, mkdir, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { isElectronLocaleSupported, pruneElectronLocales } from "./locales.mjs";

const supportedLocales = ["de-DE", "en-GB", "en-US", "es-419", "zh-TW"];

test("maps Electron locale names to supported frontend locales", () => {
  assert.equal(isElectronLocaleSupported("de.lproj", supportedLocales), true);
  assert.equal(
    isElectronLocaleSupported("de_FEMININE.lproj", supportedLocales),
    true,
  );
  assert.equal(isElectronLocaleSupported("en-US.pak", supportedLocales), true);
  assert.equal(
    isElectronLocaleSupported("es_419.lproj", supportedLocales),
    true,
  );
  assert.equal(
    isElectronLocaleSupported("zh_TW_NEUTER.lproj", supportedLocales),
    true,
  );
  assert.equal(isElectronLocaleSupported("fr.lproj", supportedLocales), false);
});

test("prunes unsupported macOS locale bundles", async (t) => {
  const temporaryRoot = await mkdtemp(
    path.join(os.tmpdir(), "chatto-desktop-locales-"),
  );
  t.after(() => rm(temporaryRoot, { recursive: true, force: true }));

  const resources = path.join(
    temporaryRoot,
    "Contents",
    "Frameworks",
    "Electron Framework.framework",
    "Versions",
    "A",
    "Resources",
  );
  await Promise.all(
    ["de.lproj", "en.lproj", "fr.lproj"].map(async (locale) => {
      const directory = path.join(resources, locale);
      await mkdir(directory, { recursive: true });
      await writeFile(path.join(directory, "locale.pak"), locale);
    }),
  );

  const result = await pruneElectronLocales(
    temporaryRoot,
    "darwin",
    supportedLocales,
  );

  assert.deepEqual(await readdir(resources), ["de.lproj", "en.lproj"]);
  assert.equal(result.removedLocales, 1);
  assert.equal(result.removedBytes, Buffer.byteLength("fr.lproj"));
});

test("prunes unsupported Windows and Linux locale packs", async (t) => {
  const temporaryRoot = await mkdtemp(
    path.join(os.tmpdir(), "chatto-desktop-locales-"),
  );
  t.after(() => rm(temporaryRoot, { recursive: true, force: true }));

  const locales = path.join(temporaryRoot, "locales");
  await mkdir(locales, { recursive: true });
  await Promise.all(
    ["de.pak", "en-US.pak", "fr.pak"].map((locale) =>
      writeFile(path.join(locales, locale), locale),
    ),
  );

  const result = await pruneElectronLocales(
    temporaryRoot,
    "linux",
    supportedLocales,
  );

  assert.deepEqual(await readdir(locales), ["de.pak", "en-US.pak"]);
  assert.equal(result.removedLocales, 1);
  assert.equal(result.removedBytes, Buffer.byteLength("fr.pak"));
});
