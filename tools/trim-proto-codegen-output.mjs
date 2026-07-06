import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '..');

const generatedFiles = new Set([
  'apps/docs-website/src/generated/connectrpc-api/admin.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/api.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/auth.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/discovery.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/realtime.raw.mdx'
]);

async function collectGeneratedTypeScript(relativeDir) {
  const dir = path.join(repoRoot, relativeDir);
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const relativePath = path.join(relativeDir, entry.name);
    if (entry.isDirectory()) {
      await collectGeneratedTypeScript(relativePath);
    } else if (entry.isFile() && entry.name.endsWith('.ts')) {
      generatedFiles.add(relativePath);
    }
  }
}

await collectGeneratedTypeScript('packages/api-types/src/chatto');

for (const generatedFile of generatedFiles) {
  const filePath = path.join(repoRoot, generatedFile);
  const content = await readFile(filePath, 'utf8');
  const normalized = `${content.trimEnd()}\n`;
  if (normalized !== content) {
    await writeFile(filePath, normalized);
  }
}
