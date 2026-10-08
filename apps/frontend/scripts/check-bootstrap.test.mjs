// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import test from 'node:test';
import { checkBootstrap } from './check-bootstrap.mjs';

// These identifiers come from the local build that failed before rendering:
// a concurrent check used the package version instead of the local build ID.
const html = '<script>__sveltekit_l1jnnl = { base: "" };</script>';

test('rejects the HTML/client mismatch that prevents SvelteKit startup', () => {
  assert.throws(
    () =>
      checkBootstrap(html, [
        { file: 'chunks/CzYo-pda.js', source: 'if(globalThis.__sveltekit_iljrdk.data){}' }
      ]),
    /CzYo-pda\.js requires __sveltekit_iljrdk, but SPA HTML defines __sveltekit_l1jnnl/
  );
});

test('accepts matching payload references across client chunks', () => {
  assert.doesNotThrow(() =>
    checkBootstrap(html, [
      { file: 'entry/start.js', source: 'export { start } from "../chunks/client.js";' },
      { file: 'chunks/client.js', source: 'if(globalThis.__sveltekit_l1jnnl.data){}' },
      { file: 'chunks/remote.js', source: 'const remote=globalThis.__sveltekit_l1jnnl.remote;' }
    ])
  );
});

test('rejects missing HTML payloads and missing client references', () => {
  assert.throws(() => checkBootstrap('<script></script>', []), /HTML has no/);
  assert.throws(() => checkBootstrap(html, []), /Client scripts have no/);
});
