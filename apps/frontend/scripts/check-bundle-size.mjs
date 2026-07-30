import { readFile, readdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const frontendRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const clientRoot = resolve(frontendRoot, '.svelte-kit/output/client');
const generatedNodesRoot = resolve(frontendRoot, '.svelte-kit/generated/client-optimized/nodes');
const manifestPath = resolve(clientRoot, '.vite/manifest.json');

const routes = [
  {
    name: 'login',
    budgetKiB: 290,
    components: ['src/routes/+layout.svelte', 'src/routes/login/+page.svelte']
  },
  {
    name: 'overview',
    budgetKiB: 370,
    additionalEntries: ['src/routes/chat/AuthenticatedRoot.svelte'],
    components: [
      'src/routes/+layout.svelte',
      'src/routes/chat/+layout.svelte',
      'src/routes/chat/[serverId]/+layout.svelte',
      'src/routes/chat/[serverId]/overview/+page.svelte'
    ]
  },
  {
    name: 'room',
    budgetKiB: 540,
    additionalEntries: ['src/routes/chat/AuthenticatedRoot.svelte'],
    components: [
      'src/routes/+layout.svelte',
      'src/routes/chat/+layout.svelte',
      'src/routes/chat/[serverId]/+layout.svelte',
      'src/routes/chat/[serverId]/[roomId]/+layout.svelte',
      'src/routes/chat/[serverId]/[roomId]/+page.svelte'
    ]
  }
];

const manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
const manifestByFile = new Map(
  Object.entries(manifest).map(([key, entry]) => [entry.file, { key, entry }])
);
const nodeEntries = await discoverNodeEntries();
const sharedEntryKeys = [
  findManifestKey((entry) => entry.name === 'entry/start'),
  findManifestKey((entry) => entry.name === 'entry/app')
];

let failed = false;
const routeResults = [];

for (const route of routes) {
  const entryKeys = [
    ...sharedEntryKeys,
    ...(route.additionalEntries ?? []).map((source) =>
      findManifestKey((entry) => entry.src === source)
    ),
    ...route.components.map((component) => {
      const nodeFile = nodeEntries.get(component);
      if (!nodeFile) throw new Error(`Could not find generated SvelteKit node for ${component}`);
      return findManifestKey((entry) => entry.src?.endsWith(`/nodes/${nodeFile}`));
    })
  ];
  const files = collectInitialFiles(entryKeys);
  const gzipBytes = await gzipFiles(files);
  const budgetBytes = route.budgetKiB * 1024;

  routeResults.push({ ...route, initialFiles: files, gzipBytes });
  if (gzipBytes > budgetBytes) failed = true;
}

const liveKitEntry = Object.entries(manifest).find(([, entry]) =>
  entry.src?.includes('/livekit-client/dist/livekit-client.esm.mjs')
);
if (!liveKitEntry || !liveKitEntry[1].isDynamicEntry) {
  throw new Error('Expected livekit-client to remain a dynamic production entry');
}
for (const result of routeResults) {
  if (result.initialFiles.has(liveKitEntry[1].file)) {
    throw new Error(`Expected livekit-client to stay outside the ${result.name} initial bundle`);
  }
}

const widestRouteNameLength = Math.max(...routeResults.map(({ name }) => name.length));
for (const result of routeResults) {
  const actual = formatKiB(result.gzipBytes).padStart(9);
  const budget = `${result.budgetKiB.toFixed(1)} KiB`.padStart(9);
  const status = result.gzipBytes <= result.budgetKiB * 1024 ? 'PASS' : 'FAIL';
  console.log(
    `${result.name.padEnd(widestRouteNameLength)}  ${actual} / ${budget}  ` +
      `${String(result.initialFiles.size).padStart(2)} initial files  ${status}`
  );
}

if (failed) {
  console.error('\nProduction bundle budget exceeded.');
  process.exitCode = 1;
}

async function discoverNodeEntries() {
  const entries = new Map();
  const filenames = await readdir(generatedNodesRoot);

  await Promise.all(
    filenames
      .filter((filename) => filename.endsWith('.js'))
      .map(async (filename) => {
        const source = await readFile(resolve(generatedNodesRoot, filename), 'utf8');
        for (const route of routes) {
          for (const component of route.components) {
            if (source.includes(component)) entries.set(component, filename);
          }
        }
      })
  );

  return entries;
}

function findManifestKey(predicate) {
  const match = Object.entries(manifest).find(([, entry]) => predicate(entry));
  if (!match) throw new Error('Could not find expected entry in the Vite manifest');
  return match[0];
}

function collectInitialFiles(entryKeys) {
  const visitedEntries = new Set();
  const files = new Set();
  const pending = [...entryKeys];

  while (pending.length > 0) {
    const key = pending.pop();
    if (visitedEntries.has(key)) continue;
    visitedEntries.add(key);

    const entry = manifest[key];
    if (!entry) throw new Error(`Could not resolve Vite manifest entry ${key}`);
    files.add(entry.file);
    for (const cssFile of entry.css ?? []) files.add(cssFile);

    for (const importReference of entry.imports ?? []) {
      const imported = manifest[importReference] ?? manifestByFile.get(importReference)?.entry;
      const importedKey =
        manifest[importReference] !== undefined
          ? importReference
          : manifestByFile.get(importReference)?.key;
      if (!imported || !importedKey) {
        throw new Error(`Could not resolve Vite manifest import ${importReference}`);
      }
      pending.push(importedKey);
    }
  }

  return files;
}

async function gzipFiles(files) {
  let total = 0;
  for (const file of files) {
    const contents = await readFile(resolve(clientRoot, file));
    total += gzipSync(contents, { level: 9 }).byteLength;
  }
  return total;
}

function formatKiB(bytes) {
  return `${(bytes / 1024).toFixed(1)} KiB`;
}
