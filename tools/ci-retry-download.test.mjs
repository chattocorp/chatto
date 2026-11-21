// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { mock, test } from 'node:test';
import { isTransientDownloadFailure, retryDownload } from './ci-retry-download.mjs';

test('classifies observed Go and Electron transport errors', () => {
  for (const output of [
    'read "https://proxy.golang.org/module.zip": stream error: stream ID 819; INTERNAL_ERROR; received from peer',
    'HTTPError: Response code 500 (Internal Server Error)',
    'HTTPError: Response code 504 (Gateway Time-out)',
    'HTTPError: Response code 429 (Too Many Requests)',
    'read ECONNRESET',
    'getaddrinfo EAI_AGAIN'
  ])
    assert.equal(isTransientDownloadFailure(output), true, output);
});

test('does not classify permanent download or code failures as transient', () => {
  for (const output of [
    'HTTPError: Response code 401 (Unauthorized)',
    'HTTPError: Response code 404 (Not Found)',
    'checksum mismatch',
    'missing go.sum entry for module providing package',
    'undefined: missingFunction',
    'AssertionError: expected 280 to be less than 2'
  ])
    assert.equal(isTransientDownloadFailure(output), false, output);
});

test('retries transport failures and stops as soon as a download succeeds', async () => {
  const run = mock.fn(() => ({ status: 0, stdout: '', stderr: '' }));
  run.mock.mockImplementationOnce(() => ({ status: 1, stderr: 'ECONNRESET' }));
  const wait = mock.fn(async () => {});
  assert.equal(await retryDownload('downloader', ['asset'], { run, wait }), 0);
  assert.equal(run.mock.callCount(), 2);
  assert.deepEqual(
    wait.mock.calls.map((call) => call.arguments),
    [[5000]]
  );
  assert.deepEqual(run.mock.calls[0].arguments.slice(0, 2), ['downloader', ['asset']]);
});

test('bounds retries and preserves the failing exit code', async () => {
  const run = mock.fn(() => ({ status: 7, stderr: 'ETIMEDOUT' }));
  const wait = mock.fn(async () => {});
  assert.equal(await retryDownload('downloader', [], { run, wait }), 7);
  assert.equal(run.mock.callCount(), 3);
  assert.deepEqual(
    wait.mock.calls.map((call) => call.arguments),
    [[5000], [10000]]
  );
});

test('stops on permanent failures, signals, and spawn errors', async () => {
  for (const result of [
    { status: 2, stderr: 'checksum mismatch' },
    { status: null, signal: 'SIGTERM', stderr: 'ECONNRESET' }
  ]) {
    const run = mock.fn(() => result);
    const wait = mock.fn(async () => {});
    assert.equal(await retryDownload('downloader', [], { run, wait }), result.status ?? 1);
    assert.equal(run.mock.callCount(), 1);
    assert.equal(wait.mock.callCount(), 0);
  }
  const error = new Error('executable unavailable');
  await assert.rejects(retryDownload('downloader', [], { run: () => ({ error }) }), error);
});
