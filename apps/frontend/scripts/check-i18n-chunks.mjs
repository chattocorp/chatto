// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

// Inspect the production module graph so an eager import cannot silently undo
// lazy management loading, including the English fallback.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { managementCatalogSections } from '../src/lib/i18n/catalogSections.js';

const manifest = JSON.parse(
  readFileSync(new URL('../.svelte-kit/output/client/.vite/manifest.json', import.meta.url), 'utf8')
);
const messages = new URL('../messages/', import.meta.url);
const expected = readdirSync(messages).filter((locale) =>
  readdirSync(new URL(`${locale}/`, messages)).some((file) =>
    managementCatalogSections.some((section) => file === `${section}.json`)
  )
);
const management = new Set(
  Object.keys(manifest).filter((key) => /^lingua-.+-management$/.test(manifest[key].name))
);
for (const locale of expected) {
  assert.ok(
    [...management].some((key) => manifest[key].name === `lingua-${locale}-management`),
    `${locale} must have a separate management translation payload`
  );
}
for (const [key, chunk] of Object.entries(manifest)) {
  for (const imported of chunk.imports ?? []) {
    assert.ok(
      !management.has(imported) || management.has(key),
      `${chunk.file} statically imports management translations from ${manifest[imported].file}`
    );
  }
}
console.log(`i18n: ${management.size} management payloads stay lazy, including en-GB  PASS`);
