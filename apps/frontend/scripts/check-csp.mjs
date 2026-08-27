// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

const html = readFileSync(new URL('../build/200.html', import.meta.url), 'utf8');
const metaMatch = /<meta\s+http-equiv="content-security-policy"\s+content="([^"]+)">/.exec(html);

if (!metaMatch) throw new Error('build/200.html has no enforcing CSP meta element');
if (metaMatch.index > html.indexOf('<script')) {
  throw new Error('the CSP meta element must precede every script');
}

const directives = new Map(
  metaMatch[1]
    .split(';')
    .map((directive) => directive.trim().split(/\s+/))
    .filter(([name]) => name)
    .map(([name, ...values]) => [name, values])
);

function requireValues(name, expected) {
  const actual = directives.get(name);
  if (!actual) throw new Error(`CSP is missing ${name}`);
  for (const value of expected) {
    if (!actual.includes(value)) throw new Error(`CSP ${name} is missing ${value}`);
  }
}

requireValues('default-src', ["'self'"]);
requireValues('script-src', ["'self'"]);
requireValues('style-src', ["'self'", "'unsafe-inline'"]);
requireValues('worker-src', ["'self'"]);
requireValues('frame-src', ['https://www.youtube-nocookie.com']);
requireValues('connect-src', ["'self'", 'http:', 'https:', 'ws:', 'wss:']);

if (directives.get('script-src').includes("'unsafe-inline'")) {
  throw new Error("CSP script-src must not contain 'unsafe-inline'");
}

for (const [, script] of html.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g)) {
  const hash = `'sha256-${createHash('sha256').update(script).digest('base64')}'`;
  if (!directives.get('script-src').includes(hash)) {
    throw new Error(`CSP does not authorize inline script ${hash}`);
  }
}

console.log('csp      enforcing resource policy  PASS');
