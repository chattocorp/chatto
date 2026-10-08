#!/usr/bin/env node
// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Run the official MCP server suite against an authenticated Chatto endpoint.
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

const child = spawn(
  'pnpm',
  [
    'dlx',
    `@modelcontextprotocol/conformance@${suiteVersion}`,
    'server',
    '--url',
    `http://${relayHost}/mcp`,
    ...(scenario
      ? ['--scenario', scenario, '--spec-version', '2026-07-28']
      : ['--requirements', '2026-07-28']),
    '--output-dir',
    outputDir
  ],
  { stdio: 'inherit' }
);
const stop = () => child.kill('SIGTERM');
process.once('SIGINT', stop);
process.once('SIGTERM', stop);
try {
  process.exitCode = await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code) => resolve(code ?? 1));
  });
} finally {
  process.removeListener('SIGINT', stop);
  process.removeListener('SIGTERM', stop);
  relay.closeAllConnections();
  await new Promise((resolve) => relay.close(resolve));
}
