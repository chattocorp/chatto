// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

import { cp, readdir, realpath, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";

const [sourceArgument, targetArgument] = process.argv.slice(2);
if (!sourceArgument || !targetArgument || process.argv.length !== 4) {
  console.error("usage: prepare-embedded-frontend.mjs <frontend-build-directory> <target-.client-directory>");
  process.exit(2);
}

const sourceDirectory = await realpath(sourceArgument);
const targetParent = await realpath(path.dirname(targetArgument));
const targetDirectory = path.join(targetParent, path.basename(targetArgument));

if (path.basename(targetDirectory) !== ".client") {
  throw new Error(`refusing to replace non-.client target: ${targetDirectory}`);
}
if (sourceDirectory === targetDirectory) {
  throw new Error("frontend source and target directories must differ");
}
if (!(await exists(path.join(sourceDirectory, "200.html"))) &&
    !(await exists(path.join(sourceDirectory, "200.html.gz")))) {
  throw new Error(`frontend build is missing 200.html or 200.html.gz: ${sourceDirectory}`);
}

await rm(targetDirectory, { recursive: true, force: true });
await cp(sourceDirectory, targetDirectory, { recursive: true });
await removeRawGzipSiblings(targetDirectory);
await writeFile(
  path.join(targetDirectory, ".gitkeep"),
  process.platform === "win32" ? "\r\n" : "\n",
);

async function exists(file) {
  try {
    await stat(file);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
}

async function removeRawGzipSiblings(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      await removeRawGzipSiblings(entryPath);
    } else if (entry.isFile() && entry.name.endsWith(".gz")) {
      const rawPath = entryPath.slice(0, -3);
      if (await exists(rawPath)) await rm(rawPath);
    }
  }
}
