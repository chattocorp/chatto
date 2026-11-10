/** Build a source-free test image with the repository's pinned tools and browser version. */
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { task, Type } from 'runling';
import { docker } from 'runling/docker';

/** Resolve image arguments from the same tool and dependency settings used by GitHub CI. */
export async function imageVersions(repository: string): Promise<Record<string, string>> {
  const mise = await readFile(join(repository, 'mise.toml'), 'utf8');
  const manifest = JSON.parse(await readFile(join(repository, 'package.json'), 'utf8')) as {
    packageManager: string;
  };
  const playwright = JSON.parse(
    await readFile(join(repository, 'apps/frontend/node_modules/playwright/package.json'), 'utf8')
  ) as { version: string };
  const version = (tool: string) => {
    const value = mise.match(new RegExp(`^${tool} = "([^"]+)"`, 'm'))?.[1];
    if (!value || !/^\d+\.\d+\.\d+$/.test(value)) throw new Error(`Missing pinned ${tool} version`);
    return value;
  };
  if (!/^pnpm@\d+\.\d+\.\d+$/.test(manifest.packageManager))
    throw new Error('Missing pinned pnpm version');
  return {
    NODE_VERSION: version('node'),
    GO_VERSION: version('go'),
    PNPM_VERSION: manifest.packageManager.slice('pnpm@'.length),
    PLAYWRIGHT_VERSION: playwright.version
  };
}

const repository = fileURLToPath(new URL('../..', import.meta.url));

/** Build once per pipeline. Use the returned immutable image ID in every test job. */
export const buildCIImage = task(
  {
    name: 'Build test image',
    input: Type.Object({}),
    result: Type.String()
  },
  async (ctx) =>
    docker.build(ctx, {
      contextDirectory: join(repository, 'ci/docker'),
      tag: 'chatto-ci:local',
      buildArgs: await imageVersions(repository),
      timeoutMs: 30 * 60_000
    })
);
