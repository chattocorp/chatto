import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '..');

const generatedFiles = [
  'apps/docs-website/src/generated/connectrpc-api/admin.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/api.raw.mdx',
  'apps/docs-website/src/generated/connectrpc-api/realtime.raw.mdx',
  'packages/api-types/src/chatto/admin/v1/server_connect.ts',
  'packages/api-types/src/chatto/admin/v1/server_pb.ts'
];

for (const generatedFile of generatedFiles) {
  const filePath = path.join(repoRoot, generatedFile);
  const content = await readFile(filePath, 'utf8');
  const normalized = `${content.trimEnd()}\n`;
  if (normalized !== content) {
    await writeFile(filePath, normalized);
  }
}
