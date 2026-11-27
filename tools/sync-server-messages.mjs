// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

// Copies the server sections of the frontend translations into the Go module.
//
// Server catalogs live with the other translations in
// apps/frontend/messages/<locale>/server/*.json. Go can embed only files in its
// module, so this script mirrors them to cli/internal/i18n/messages/<locale>/.
// It also removes copies whose source no longer exists. A Go test in
// cli/internal/i18n fails when the two trees differ.
//
// Usage: mise sync-server-messages

import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = path.join(root, 'apps/frontend/messages');
const target = path.join(root, 'cli/internal/i18n/messages');

rmSync(target, { recursive: true, force: true });
let count = 0;
for (const locale of readdirSync(source).sort()) {
  const dir = path.join(source, locale, 'server');
  if (!existsSync(dir)) continue;
  for (const file of readdirSync(dir).filter((name) => name.endsWith('.json'))) {
    mkdirSync(path.join(target, locale), { recursive: true });
    cpSync(path.join(dir, file), path.join(target, locale, file));
    count++;
  }
}
console.log(`Copied ${count} server catalog files to ${path.relative(root, target)}`);
