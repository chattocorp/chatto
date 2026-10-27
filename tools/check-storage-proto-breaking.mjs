import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// ADR-084 renames the event-value file and relocates the retained RoomLayout
// values to runtime_state. Historical and current-main descriptor fixtures must
// pass first; they check the stored fields even when Buf sees a deleted file.
const oldEventValuesPath = 'chatto/core/evt/v1/models.proto';

/**
 * Accept only the two reviewed source changes from the storage-contract cleanup.
 * @param {{path?: string, type: string, message: string}} diagnostic Buf diagnostic.
 */
export function isReviewedStorageRelocation(diagnostic) {
  return (
    (diagnostic.type === 'FILE_NO_DELETE' &&
      (diagnostic.path === undefined || diagnostic.path === oldEventValuesPath) &&
      diagnostic.message === `Previously present file "${oldEventValuesPath}" was deleted.`) ||
    (diagnostic.path === 'chatto/core/runtime_state/v1/legacy_state.proto' &&
      diagnostic.type === 'FIELD_SAME_TYPE' &&
      diagnostic.message ===
        'Field "1" with name "legacy_sections" on message "RoomLayout" changed type from "chatto.core.evt.v1.RoomGroup" to "chatto.core.runtime_state.v1.RoomGroup".')
  );
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
    const failures = diagnostics.filter((diagnostic) => !isReviewedStorageRelocation(diagnostic));
    for (const diagnostic of diagnostics) {
      if (isReviewedStorageRelocation(diagnostic)) {
        console.log(`Accepted storage-contract relocation: ${diagnostic.message}`);
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
