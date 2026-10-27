import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// These runtime-only messages were removed by the read-model split (ADR-084).
// The historical stored-schema fixtures must pass before this exception applies.
// Keep all other changes in models.proto subject to the normal Buf checks.
const removedReadModels = new Set(['Role', 'Room', 'RoomMembership', 'RoomNotice', 'User']);

/**
 * Accept only the exact source deletions from the internal read-model split.
 * @param {{path: string, type: string, message: string}} diagnostic Buf diagnostic.
 */
export function isReadModelDeletion(diagnostic) {
  if (
    diagnostic.path !== 'chatto/core/evt/v1/models.proto' ||
    diagnostic.type !== 'MESSAGE_NO_DELETE'
  ) {
    return false;
  }
  const match = /^Previously present message "([^"]+)" was deleted from file\.$/.exec(
    diagnostic.message
  );
  return match !== null && removedReadModels.has(match[1]);
}

function runCompatibilityTest(repoDir) {
  const result = spawnSync('go', ['test', './internal/protocompat', '-count=1'], {
    cwd: path.join(repoDir, 'cli'),
    encoding: 'utf8'
  });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

function main() {
  const against = process.argv[2];
  if (!against) {
    console.error('usage: check-storage-proto-breaking.mjs <against-input>');
    process.exit(2);
  }

  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const repoDir = path.resolve(scriptDir, '..');
  const protoDir = path.join(repoDir, 'proto');

  runCompatibilityTest(repoDir);

  const result = spawnSync(
    'buf',
    [
      'breaking',
      '.',
      '--against',
      against,
      '--error-format=json',
      '--exclude-imports',
      '--exclude-path',
      'chatto/auth/v1',
      '--exclude-path',
      'chatto/discovery/v1',
      '--exclude-path',
      'chatto/api/v1',
      '--exclude-path',
      'chatto/admin/v1',
      '--exclude-path',
      'chatto/realtime/v1',
      '--exclude-path',
      'chatto/core/live/v1',
      '--exclude-path',
      'chatto/core/pubsub/v1',
      '--exclude-path',
      'chatto/core/projection/v1'
    ],
    { cwd: protoDir, encoding: 'utf8' }
  );

  if (result.error) throw result.error;
  if (result.stderr) process.stderr.write(result.stderr);
  // Buf uses status 100 for schema differences. Fail closed for command errors,
  // malformed diagnostics, or a failure without diagnostic output.
  if (result.status === 100 && result.stdout.trim()) {
    const diagnostics = result.stdout
      .trim()
      .split('\n')
      .map((line) => JSON.parse(line));
    const failures = diagnostics.filter((diagnostic) => !isReadModelDeletion(diagnostic));
    for (const diagnostic of diagnostics) {
      if (isReadModelDeletion(diagnostic)) {
        console.log(`Accepted internal read-model deletion: ${diagnostic.message}`);
      }
    }
    for (const diagnostic of failures) console.error(JSON.stringify(diagnostic));
    process.exit(failures.length === 0 ? 0 : 100);
  }
  if (result.stdout) process.stdout.write(result.stdout);
  process.exit(result.status ?? 1);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
