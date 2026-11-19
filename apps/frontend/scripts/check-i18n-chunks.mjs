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
  Object.keys(manifest).filter((key) => /^translatto-.+-management$/.test(manifest[key].name))
);
for (const locale of expected) {
  assert.ok(
    [...management].some((key) => manifest[key].name === `translatto-${locale}-management`),
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

// Help must remain separate even from management and the English shell.
const help = new Set(
  Object.keys(manifest).filter((key) => /^translatto-.+-help$/.test(manifest[key].name))
);
for (const locale of readdirSync(messages)) {
  assert.ok(
    [...help].some((key) => manifest[key].name === `translatto-${locale}-help`),
    `${locale} must have a separate help translation payload`
  );
}
for (const [key, chunk] of Object.entries(manifest)) {
  for (const imported of chunk.imports ?? []) {
    assert.ok(
      !help.has(imported) || help.has(key),
      `${chunk.file} statically imports help translations from ${manifest[imported].file}`
    );
  }
}
console.log(`i18n: ${help.size} help payloads stay lazy, including en-GB  PASS`);

// Every authored article must be a dynamic entry, never part of another route's
// static graph. The build also compiles documents in languages not visited in tests.
const content = new URL('../src/lib/help/content/', import.meta.url);
const articleKeys = new Set();
for (const locale of readdirSync(content)) {
  for (const file of readdirSync(new URL(`${locale}/`, content))) {
    if (!file.endsWith('.mdx')) continue;
    const key = `src/lib/help/content/${locale}/${file}`;
    assert.ok(manifest[key]?.isDynamicEntry, `${key} must stay independently lazy`);
    articleKeys.add(key);
  }
}
for (const chunk of Object.values(manifest)) {
  for (const imported of chunk.imports ?? []) {
    assert.ok(!articleKeys.has(imported), `${chunk.file} statically imports ${imported}`);
  }
}
console.log(`help: ${articleKeys.size} compiled articles stay lazy  PASS`);
