/** Install the release tarball outside the workspace and check its runtime and declarations. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { delimiter, dirname, resolve } from 'node:path';

process.env.PATH = `${dirname(process.execPath)}${delimiter}${process.env.PATH ?? ''}`;
const root = resolve(import.meta.dirname, '..');
const temporary = await mkdtemp(resolve(tmpdir(), 'chatto-client-consumer-'));
try {
  execFileSync('pnpm', ['pack', '--pack-destination', temporary], { cwd: root, stdio: 'inherit' });
  const archives = (await readdir(temporary)).filter((file) => file.endsWith('.tgz'));
  assert.equal(archives.length, 1);
  const archive = resolve(temporary, archives[0]);
  const listing = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' });
  assert(!/\.(test|spec|svelte)\.|\/testing\//.test(listing));
  assert(listing.includes('api-types/LICENSE'));
  const consumer = resolve(temporary, 'consumer with spaces');
  await mkdir(consumer);
  await writeFile(
    resolve(consumer, 'package.json'),
    JSON.stringify({ private: true, type: 'module' })
  );
  execFileSync(
    'npm',
    ['install', '--ignore-scripts', '--no-audit', '--no-fund', '--omit=optional', archive],
    { cwd: consumer, stdio: 'inherit' }
  );
  const installed = resolve(consumer, 'node_modules/@chatto/client');
  const manifest = JSON.parse(await readFile(resolve(installed, 'package.json'), 'utf8'));
  const source = JSON.parse(await readFile(resolve(root, 'package.json'), 'utf8'));
  assert.equal(manifest.name, source.name);
  assert.equal(manifest.version, source.version);
  assert.equal(manifest.license, 'Apache-2.0');
  for (const file of ['LICENSE', 'api-types/LICENSE']) {
    assert.equal(
      await readFile(resolve(installed, file), 'utf8'),
      await readFile(resolve(root, '../../LICENSES/Apache-2.0.txt'), 'utf8')
    );
  }
  assert.equal(
    await readFile(resolve(installed, 'NOTICE'), 'utf8'),
    await readFile(resolve(root, 'NOTICE'), 'utf8')
  );
  assert.equal(manifest.dependencies['@chatto/api-types'], undefined);
  assert.equal(manifest.private, undefined);
  assert.deepEqual(manifest.sideEffects, ['./chatto-client/src/svelte/index.js']);
  // Import every compiled module, including deep exports and the optional Svelte adapter.
  execFileSync(
    process.execPath,
    [
      '--input-type=module',
      '-e',
      "import { createClient } from '@chatto/client'; createClient().close();"
    ],
    { cwd: consumer, stdio: 'inherit', timeout: 30000 }
  );
  execFileSync('npm', ['install', '--ignore-scripts', '--no-audit', '--no-fund', 'svelte@^5.7.0'], {
    cwd: consumer,
    stdio: 'inherit'
  });
  const modules = (
    await readdir(resolve(installed, 'chatto-client/src'), { recursive: true })
  ).filter((file) => file.endsWith('.js'));
  await writeFile(
    resolve(consumer, 'smoke.mjs'),
    `
import assert from 'node:assert/strict';
import { createClient, createApi } from '@chatto/client';
import { UserService } from '@chatto/client/types';
import { signal } from '@chatto/client/reactivity';
assert(UserService.typeName);
assert.equal(typeof createApi, 'function');
const client = createClient();
client.close();
assert.equal(signal(42).get(), 42);
for (const path of ${JSON.stringify(modules)}) {
  await import('@chatto/client/' + path.slice(0, -3));
}
`
  );
  execFileSync(process.execPath, ['smoke.mjs'], {
    cwd: consumer,
    stdio: 'inherit',
    timeout: 30000
  });
  await writeFile(
    resolve(consumer, 'types.ts'),
    `
import { createClient } from '@chatto/client';
import { UserService } from '@chatto/client/types';
import '@chatto/client/types/admin';
import '@chatto/client/svelte';
const client = createClient();
const name: string = UserService.typeName;
client.close();
void name;
`
  );
  await writeFile(
    resolve(consumer, 'tsconfig.json'),
    JSON.stringify({
      compilerOptions: {
        strict: true,
        noEmit: true,
        module: 'NodeNext',
        target: 'ES2022',
        lib: ['ES2024', 'DOM', 'DOM.Iterable'],
        types: []
      },
      include: ['types.ts']
    })
  );
  execFileSync(
    process.execPath,
    [resolve(root, 'node_modules/typescript/bin/tsc'), '-p', consumer],
    { cwd: consumer, stdio: 'inherit' }
  );
  if (process.env.CHATTO_CLIENT_RELEASE_DIR) {
    await mkdir(process.env.CHATTO_CLIENT_RELEASE_DIR, { recursive: true });
    await copyFile(archive, resolve(process.env.CHATTO_CLIENT_RELEASE_DIR, archives[0]));
  }
  console.log(
    `Verified ${manifest.name}@${manifest.version}: tarball, Node ESM, and declarations.`
  );
} finally {
  await rm(temporary, { recursive: true, force: true });
}
