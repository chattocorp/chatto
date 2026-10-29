// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { APP_ORIGIN } from './frontend_protocol.mjs';
import { configureLastServer } from './last_server.mjs';

function fixture(t) {
  const directory = mkdtempSync(path.join(tmpdir(), 'chatto-last-server-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const file = path.join(directory, 'last-server.json');
  const contents = new EventEmitter();
  return { file, contents };
}

test('first launch uses the directory; a new window restores the last server root', (t) => {
  const { file, contents } = fixture(t);
  assert.equal(configureLastServer(contents, file), APP_ORIGIN);
  contents.emit('did-navigate', {}, `${APP_ORIGIN}/chat/one.example/overview`);
  contents.emit(
    'did-navigate-in-page',
    {},
    `${APP_ORIGIN}/chat/two.example/room-1/thread-2?search=private#message`,
    true
  );
  assert.equal(readFileSync(file, 'utf8'), '"/chat/two.example"');
  assert.equal(configureLastServer(new EventEmitter(), file), `${APP_ORIGIN}/chat/two.example`);
});

test('global pages, authentication, external URLs, and subframes do not replace the server', (t) => {
  const { file, contents } = fixture(t);
  configureLastServer(contents, file);
  contents.emit('did-navigate', {}, `${APP_ORIGIN}/chat/one.example/room-1`);
  for (const url of [
    `${APP_ORIGIN}/chat`,
    `${APP_ORIGIN}/chat/servers`,
    `${APP_ORIGIN}/chat/notifications`,
    `${APP_ORIGIN}/chat/preferences`,
    `${APP_ORIGIN}/servers/callback?code=secret`,
    `${APP_ORIGIN}/login`,
    'https://external.example/chat/two.example',
    'invalid'
  ])
    contents.emit('did-navigate', {}, url);
  contents.emit('did-navigate-in-page', {}, `${APP_ORIGIN}/chat/iframe.example`, false);
  assert.equal(configureLastServer(new EventEmitter(), file), `${APP_ORIGIN}/chat/one.example`);
});

test('invalid stored preferences fall back to the directory', (t) => {
  const { file } = fixture(t);
  for (const value of [
    'broken json',
    'null',
    '{}',
    '"https://external.example"',
    '"/servers/callback"'
  ]) {
    writeFileSync(file, value);
    assert.equal(configureLastServer(new EventEmitter(), file), APP_ORIGIN);
  }
});

test('unavailable storage does not interrupt navigation or startup', (t) => {
  const { file, contents } = fixture(t);
  assert.equal(configureLastServer(contents, path.join(file, 'missing')), APP_ORIGIN);
  assert.doesNotThrow(() => contents.emit('did-navigate', {}, `${APP_ORIGIN}/chat/one.example`));
});
