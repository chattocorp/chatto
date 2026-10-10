#!/usr/bin/env node
// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Run official MCP application protocol checks against a Chatto endpoint.
 * The full SDK fixture suite is an explicit diagnostic option, not the default.
 * The suite has no bearer-header option. A temporary loopback relay adds the
 * credential from a file, while the production endpoint still validates it.
 * This relay does not verify OAuth or the target's Host/Origin protection.
 */
import { spawn } from 'node:child_process';
import { readFile, mkdir } from 'node:fs/promises';
import http from 'node:http';
import https from 'node:https';
import path from 'node:path';

const suiteVersion = '0.2.0-alpha.12';
const scenario = process.env.CHATTO_MCP_SCENARIO;
const fullSuite = process.env.CHATTO_MCP_FULL_SUITE;
if (fullSuite !== undefined && fullSuite !== '1') {
  throw new Error('CHATTO_MCP_FULL_SUITE must be 1 or unset.');
}
if (fullSuite && scenario) {
  throw new Error('Choose CHATTO_MCP_FULL_SUITE or CHATTO_MCP_SCENARIO, not both.');
}
// These scenarios discover the application's real tools. Fixture-dependent
// scenarios cannot establish whether Chatto's catalog works. Header validation
// is pending in the upstream requirement set, but is useful application coverage.
const runs = fullSuite
  ? [['--requirements', '2026-07-28']]
  : (scenario ? [scenario] : ['tools-list', 'http-header-validation']).map((name) => [
      '--scenario',
      name,
      '--spec-version',
      '2026-07-28'
    ]);
let target;
try {
  target = new URL(
    process.env.CHATTO_MCP_URL ??
      `${process.env.CHATTO_DEV_CHATTO_URL ?? 'http://localhost:4000'}/mcp`
  );
} catch {
  throw new Error('CHATTO_MCP_URL must be a valid MCP URL.');
}
const tokenFile = process.env.CHATTO_MCP_TOKEN_FILE;
if (
  !['http:', 'https:'].includes(target.protocol) ||
  target.pathname !== '/mcp' ||
  target.username ||
  target.password ||
  target.search ||
  target.hash ||
  !tokenFile
) {
  throw new Error(
    'Set CHATTO_MCP_URL to an MCP URL and CHATTO_MCP_TOKEN_FILE to a credential file.'
  );
}
const token = (await readFile(tokenFile, 'utf8')).trim();
if (!token || /\s/.test(token)) {
  throw new Error('The credential file must contain one bearer credential.');
}
const outputDir = path.resolve(
  process.env.CHATTO_MCP_RESULTS_DIR ?? '.context/mcp-conformance-results'
);
await mkdir(outputDir, { recursive: true, mode: 0o700 });

// Keep the relay on loopback and reject requests from browser origins. The
// credential never enters the command line, suite environment, or diagnostics.
let admission = Promise.resolve();
const relay = http.createServer(async (request, response) => {
  if (request.url !== '/mcp' || request.headers.host !== relayHost || request.headers.origin) {
    response.writeHead(403).end();
    return;
  }
  // The suite sends bursts without rate-limit recovery. Space requests below
  // Chatto's admission limit, without changing the production limit.
  admission = admission.then(() => new Promise((resolve) => setTimeout(resolve, 100)));
  await admission;
  if (response.destroyed) return;
  const upstream = (target.protocol === 'https:' ? https : http).request(target, {
    method: request.method,
    headers: { ...request.headers, host: target.host, authorization: `Bearer ${token}` },
    timeout: 20_000
  });
  upstream.on('response', (result) => {
    response.writeHead(result.statusCode ?? 502, result.headers);
    result.pipe(response);
  });
  upstream.on('error', () => {
    if (!response.headersSent) response.writeHead(502);
    response.end();
  });
  upstream.on('timeout', () => upstream.destroy());
  request.on('aborted', () => upstream.destroy());
  response.on('close', () => upstream.destroy());
  request.pipe(upstream);
});
await new Promise((resolve, reject) => {
  relay.once('error', reject);
  relay.listen(0, '127.0.0.1', resolve);
});
const relayHost = `127.0.0.1:${relay.address().port}`;

let child;
let stopped = false;
const stop = () => {
  stopped = true;
  child?.kill('SIGTERM');
};
process.once('SIGINT', stop);
process.once('SIGTERM', stop);
try {
  console.log(
    fullSuite
      ? 'Running the full SDK fixture suite for diagnostics; Chatto does not provide its fixtures.'
      : 'Running selected application protocol checks; this is not full MCP conformance.'
  );
  process.exitCode = 0;
  for (const selection of runs) {
    if (stopped) break;
    child = spawn(
      'pnpm',
      [
        'dlx',
        `@modelcontextprotocol/conformance@${suiteVersion}`,
        'server',
        '--url',
        `http://${relayHost}/mcp`,
        ...selection,
        '--output-dir',
        outputDir
      ],
      { stdio: 'inherit' }
    );
    const code = await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', (code) => resolve(code ?? 1));
    });
    // Run both checks, but never let a later success hide an earlier failure.
    if (code !== 0) process.exitCode = code;
  }
  if (stopped) process.exitCode = 1;
} finally {
  process.removeListener('SIGINT', stop);
  process.removeListener('SIGTERM', stop);
  relay.closeAllConnections();
  await new Promise((resolve) => relay.close(resolve));
}
