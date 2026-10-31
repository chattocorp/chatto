/** Package the updater without pnpm symlinks, and preserve dependency licences. */
import { build } from 'esbuild';
import { existsSync } from 'node:fs';
import { readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** Write the runtime and licence text into the app before ASAR creation and signing. */
export async function bundleUpdater(buildPath) {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const result = await build({
    absWorkingDir: root,
    entryPoints: ['updater_vendor.mjs'],
    outfile: path.join(buildPath, 'updater-vendor.cjs'),
    bundle: true,
    platform: 'node',
    format: 'cjs',
    external: ['electron'],
    metafile: true
  });
  const packages = new Set();
  for (const input of Object.keys(result.metafile.inputs)) {
    if (!input.includes('node_modules')) continue;
    let directory = path.dirname(path.resolve(root, input));
    while (!existsSync(path.join(directory, 'package.json'))) {
      const parent = path.dirname(directory);
      if (parent === directory) throw new Error('Cannot locate bundled dependency licence');
      directory = parent;
    }
    packages.add(directory);
  }
  const notices = [];
  for (const directory of [...packages].sort()) {
    const { name, version, license, author } = JSON.parse(
      await readFile(path.join(directory, 'package.json'), 'utf8')
    );
    const files = (await readdir(directory)).filter((name) =>
      /^(licen[cs]e|copying|notice)(\.|$)/i.test(name)
    );
    if (files.length === 0) {
      // lazy-val declares MIT and its author but publishes no licence file.
      if (name !== 'lazy-val' || license !== 'MIT' || typeof author !== 'string') {
        throw new Error(`Missing licence for bundled dependency ${name}`);
      }
      const mit = await readFile(path.join(root, '../../LICENSES/MIT.txt'), 'utf8');
      notices.push(
        `${name} ${version}\nAuthor: ${author}\n${mit.replace('Copyright (c) <year> <copyright holders>\n', '')}`
      );
    }
    for (const file of files)
      notices.push(`${name} ${version}\n${await readFile(path.join(directory, file), 'utf8')}`);
  }
  await writeFile(path.join(buildPath, 'updater-licenses.txt'), notices.join('\n\n'));
}
