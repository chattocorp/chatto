// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

// Check the adapted SPA output: concurrent SvelteKit sync commands can give
// the HTML and client different payload identifiers, which prevents startup.
import { readdirSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

/**
 * Require the client payload identifier to match the SPA bootstrap script.
 * @param {string} html The generated SPA fallback.
 * @param {Iterable<{ file: string, source: string }>} scripts Generated client scripts.
 * @throws {Error} If the payload is missing or uses different identifiers.
 */
export function checkBootstrap(html, scripts) {
  const identifier = /\b(__sveltekit_[a-z0-9]+)\s*=/.exec(html)?.[1];
  if (!identifier) throw new Error('SPA HTML has no SvelteKit payload identifier');

  let found = false;
  for (const { file, source } of scripts) {
    for (const [, required] of source.matchAll(/\bglobalThis\.(__sveltekit_[a-z0-9]+)\b/g)) {
      if (required !== identifier) {
        throw new Error(
          `${file} requires ${required}, but SPA HTML defines ${identifier}. ` +
            'Run frontend sync, checks, tests, and builds in sequence, then rebuild.'
        );
      }
      found = true;
    }
  }
  if (!found) throw new Error('Client scripts have no SvelteKit payload reference');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const buildRoot = fileURLToPath(new URL('../build/', import.meta.url));
  const clientRoot = resolve(buildRoot, '_app/immutable');
  const scripts = readdirSync(clientRoot, { recursive: true, encoding: 'utf8' })
    .filter((file) => file.endsWith('.js'))
    .map((file) => ({ file, source: readFileSync(resolve(clientRoot, file), 'utf8') }));
  checkBootstrap(readFileSync(resolve(buildRoot, '200.html'), 'utf8'), scripts);
  console.log('bootstrap HTML and client payload match  PASS');
}
