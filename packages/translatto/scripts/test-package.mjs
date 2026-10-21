/** Pack with pnpm's export overrides, then test the exact tarball outside the workspace. */
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { copyFile, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { delimiter, dirname, resolve } from 'node:path';
import { promisify } from 'node:util';

const exec = promisify(execFile);
process.env.PATH = `${dirname(process.execPath)}${delimiter}${process.env.PATH ?? ''}`;
const root = resolve(import.meta.dirname, '..');
const temporary = await mkdtemp(resolve(tmpdir(), 'translatto-consumer-'));
try {
  await exec('pnpm', ['pack', '--pack-destination', temporary], { cwd: root });
  const archives = (await readdir(temporary)).filter((file) => file.endsWith('.tgz'));
  assert.equal(archives.length, 1);
  const archive = resolve(temporary, archives[0]);
  const { stdout: listing } = await exec('tar', ['-tzf', archive]);
  const files = listing
    .trim()
    .split('\n')
    .map((file) => file.replace(/^package\//, ''));
  for (const file of ['dist/index.js', 'dist/index.d.ts', 'README.md', 'LICENSE', 'package.json']) {
    assert(files.includes(file), `Missing ${file}`);
  }
  assert(files.every((file) => !/\.(spec|test)\./.test(file) && !file.startsWith('src/')));
  const consumer = resolve(temporary, 'consumer with spaces');
  await mkdir(consumer);
  await writeFile(
    resolve(consumer, 'package.json'),
    JSON.stringify({ private: true, type: 'module' })
  );
  await exec(
    'npm',
    [
      'install',
      '--offline',
      '--cache',
      resolve(temporary, 'npm-cache'),
      '--ignore-scripts',
      '--no-audit',
      '--no-fund',
      archive
    ],
    {
      cwd: consumer
    }
  );
  const installed = resolve(consumer, 'node_modules/@chatto/translatto');
  const manifest = JSON.parse(await readFile(resolve(installed, 'package.json'), 'utf8'));
  const source = JSON.parse(await readFile(resolve(root, 'package.json'), 'utf8'));
  assert.equal(manifest.name, source.name);
  assert.equal(manifest.version, source.version);
  assert.equal(manifest.private, undefined);
  assert.equal(manifest.types, './dist/index.d.ts');
  assert.deepEqual(manifest.exports, {
    '.': { types: './dist/index.d.ts', import: './dist/index.js' },
    './package.json': './package.json'
  });
  for (const file of [
    manifest.types,
    manifest.exports['.'].import,
    manifest.exports['./package.json']
  ]) {
    await readFile(resolve(installed, file));
  }
  await writeFile(
    resolve(consumer, 'smoke.mjs'),
    `
import assert from 'node:assert/strict';
import { createTranslatto } from '@chatto/translatto';
const t = createTranslatto({
  baseLocale: 'en',
  loaders: { common: { en: async () => ({ common: {
    hello: 'Hello {name}', member_count: { one: '{count} member', other: '{count} members' }
  } }) } }
});
await t.setActiveSections(['common']);
assert.equal(t.t('common.hello', { name: 'World' }), 'Hello World');
assert.equal(t.t('common.member_count', { count: 1 }), '1 member');
assert.equal(t.t('common.member_count', { count: 2 }), '2 members');
`
  );
  await exec(process.execPath, ['smoke.mjs'], { cwd: consumer });
  // Reuse the API type contract, but resolve imports from the installed declarations.
  const types = (await readFile(resolve(root, 'test/types.ts'), 'utf8')).replace(
    "'../src/index.js'",
    "'@chatto/translatto'"
  );
  await writeFile(resolve(consumer, 'types.ts'), types);
  await writeFile(
    resolve(consumer, 'tsconfig.json'),
    JSON.stringify({
      compilerOptions: {
        strict: true,
        noEmit: true,
        module: 'NodeNext',
        target: 'ES2022',
        types: []
      },
      include: ['types.ts']
    })
  );
  await exec(process.execPath, [resolve(root, 'node_modules/typescript/bin/tsc'), '-p', consumer], {
    cwd: consumer
  });
  // Save only after all checks pass. The publish job must use this exact artifact.
  if (process.env.TRANSLATTO_RELEASE_DIR) {
    await mkdir(process.env.TRANSLATTO_RELEASE_DIR, { recursive: true });
    await copyFile(archive, resolve(process.env.TRANSLATTO_RELEASE_DIR, archives[0]));
  }
  console.log(
    `Verified ${manifest.name}@${manifest.version}: tarball, Node ESM, and declarations.`
  );
} finally {
  await rm(temporary, { recursive: true, force: true });
}
