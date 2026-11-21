// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: EUPL-1.2

/** Retry dependency downloads after transport failures. Do not wrap test commands. */
import { spawnSync } from 'node:child_process';
import { setTimeout } from 'node:timers/promises';
import { pathToFileURL } from 'node:url';

/** Match transient download errors without retrying build or checksum failures. */
export function isTransientDownloadFailure(output) {
  return /stream error: stream ID \d+; INTERNAL_ERROR; received from peer|HTTPError: Response code (?:408|429|5\d\d)\b|\b(?:ECONNRESET|ETIMEDOUT|EAI_AGAIN)\b/.test(
    output
  );
}

/** Retry a repeatable command's dependency downloads; retain its final exit code. */
export async function retryDownload(command, args, { run = spawnSync, wait = setTimeout } = {}) {
  for (let attempt = 1; attempt <= 3; attempt++) {
    const result = run(command, args, {
      encoding: 'utf8',
      maxBuffer: 16 * 1024 * 1024
    });
    if (result.error) throw result.error;
    process.stdout.write(result.stdout ?? '');
    process.stderr.write(result.stderr ?? '');
    if (result.status === 0) return 0;
    if (
      result.signal ||
      attempt === 3 ||
      !isTransientDownloadFailure(`${result.stdout}\n${result.stderr}`)
    ) {
      return result.status ?? 1;
    }
    console.error(`Dependency download failed in transport; retrying attempt ${attempt + 1}/3.`);
    await wait(attempt * 5000);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [command, ...args] = process.argv.slice(2);
  if (!command) throw new Error('Usage: ci-retry-download.mjs <command> [args...]');
  process.exitCode = await retryDownload(command, args);
}
