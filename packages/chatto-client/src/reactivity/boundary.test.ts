/// <reference types="node" />
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { expect, test } from 'vitest';

const root = join(import.meta.dirname, '..');

function sourceFiles(directory: string): string[] {
  return readdirSync(directory).flatMap((name) => {
    const path = join(directory, name);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return /\.ts$/.test(name) && !/\.(test|spec)\.ts$/.test(name) ? [path] : [];
  });
}

/**
 * The client core must stay framework-neutral: only the Svelte adapter may
 * import Svelte, and no module may use runes or frontend aliases.
 */
test('the client core has no Svelte or frontend dependency', () => {
  const violations = sourceFiles(root)
    .filter((path) => !relative(root, path).startsWith('svelte/'))
    .flatMap((path) => {
      const source = readFileSync(path, 'utf8');
      const problems: string[] = [];
      if (/from ['"](svelte|@tanstack\/svelte-query)[/'"]/.test(source))
        problems.push('svelte import');
      if (/from ['"]\$(lib|app)\//.test(source)) problems.push('frontend alias');
      if (/\$(state|derived|effect|props)\s*[(.<]/.test(source)) problems.push('rune');
      return problems.map((problem) => `${relative(root, path)}: ${problem}`);
    });
  expect(violations).toEqual([]);
});
