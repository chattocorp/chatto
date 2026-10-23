/** Build an independent npm package, including the private generated protocol modules. */
import { execFileSync } from 'node:child_process';
import { copyFile, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { dirname, relative, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const dist = resolve(root, 'dist');
await rm(dist, { recursive: true, force: true });
execFileSync(
  process.execPath,
  [resolve(root, 'node_modules/typescript/bin/tsc'), '-p', resolve(root, 'tsconfig.build.json')],
  { stdio: 'inherit' }
);
const api = JSON.parse(await readFile(resolve(root, '../api-types/package.json'), 'utf8'));
for (const entry of await readdir(dist, { recursive: true, withFileTypes: true })) {
  if (!entry.isFile() || !/\.(js|ts)$/.test(entry.name)) continue;
  const file = resolve(entry.parentPath, entry.name);
  const source = await readFile(file, 'utf8');
  // Resolve both JavaScript and declaration imports to the same bundled modules.
  const rewritten = source.replace(
    /(['"])@chatto\/api-types\/([^'"]+)\1/g,
    (_match, quote, subpath) => {
      for (const [key, target] of Object.entries(api.exports)) {
        const prefix = key.replace(/^\.\//, '').replace(/\*$/, '');
        if (!key.endsWith('*') || !subpath.startsWith(prefix)) continue;
        const output = target.replace('*', subpath.slice(prefix.length)).replace(/\.ts$/, '.js');
        const path = relative(dirname(file), resolve(dist, 'api-types', output))
          .split('\\')
          .join('/');
        return `${quote}${path.startsWith('.') ? path : `./${path}`}${quote}`;
      }
      throw new Error(`Unknown protocol import: ${subpath}`);
    }
  );
  await writeFile(file, rewritten);
}
const manifest = JSON.parse(await readFile(resolve(root, 'package.json'), 'utf8'));
delete manifest.scripts;
delete manifest.devDependencies;
delete manifest.dependencies['@chatto/api-types'];
manifest.publishConfig = { access: 'public', registry: 'https://registry.npmjs.org' };
manifest.files = ['chatto-client', 'api-types', 'README.md', 'LICENSE', 'NOTICE'];
manifest.sideEffects = ['./chatto-client/src/svelte/index.js'];
manifest.types = './chatto-client/src/index.d.ts';
manifest.exports = Object.fromEntries(
  Object.entries(manifest.exports).map(([key, target]) => [
    key,
    target.endsWith('.ts')
      ? {
          types: target.replace('./src/', './chatto-client/src/').replace(/\.ts$/, '.d.ts'),
          import: target.replace('./src/', './chatto-client/src/').replace(/\.ts$/, '.js')
        }
      : target
  ])
);
await writeFile(resolve(dist, 'package.json'), JSON.stringify(manifest, null, 2) + '\n');
for (const file of ['README.md', 'LICENSE', 'NOTICE'])
  await copyFile(resolve(root, file), resolve(dist, file));
await mkdir(resolve(dist, 'api-types'), { recursive: true });
await copyFile(resolve(root, '../../LICENSES/Apache-2.0.txt'), resolve(dist, 'api-types/LICENSE'));
