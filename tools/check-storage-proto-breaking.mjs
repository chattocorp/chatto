import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

// These messages were transient LiveEvent payloads that were incorrectly
// declared in the durable user-events file. They were never stored in EVT.
const allowedTransientMessages = new Set([
  "ServerUserPreferencesUpdatedEvent",
  "UserCreatedEvent",
  "UserDeletedEvent",
  "UserProfileUpdatedEvent",
]);

export function isAllowedStorageBreakingDiagnostic(diagnostic) {
  if (
    diagnostic.path !== "chatto/core/v1/user_events.proto" ||
    diagnostic.type !== "MESSAGE_NO_DELETE"
  ) {
    return false;
  }
  const match = diagnostic.message.match(
    /^Previously present message "([^"]+)" was deleted from file\.$/,
  );
  return Boolean(match && allowedTransientMessages.has(match[1]));
}

function main() {
  const against = process.argv[2];
  if (!against) {
    console.error("usage: check-storage-proto-breaking.mjs <against-input>");
    process.exit(2);
  }

  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const protoDir = path.resolve(scriptDir, "../proto");
  const result = spawnSync(
    "buf",
    [
      "breaking",
      ".",
      "--against",
      against,
      "--exclude-imports",
      "--exclude-path",
      "chatto/auth/v1",
      "--exclude-path",
      "chatto/discovery/v1",
      "--exclude-path",
      "chatto/api/v1",
      "--exclude-path",
      "chatto/admin/v1",
      "--exclude-path",
      "chatto/realtime/v1",
      "--exclude-path",
      "chatto/core/v1/live_events.proto",
      "--exclude-path",
      "chatto/core/v1/projection_snapshots.proto",
      "--error-format=json",
    ],
    { cwd: protoDir, encoding: "utf8" },
  );

  if (result.error) {
    throw result.error;
  }
  if (result.stderr) {
    process.stderr.write(result.stderr);
  }
  if (result.status === 0) {
    if (result.stdout) process.stdout.write(result.stdout);
    process.exit(0);
  }

  let diagnostics;
  try {
    diagnostics = result.stdout
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line));
  } catch {
    process.stdout.write(result.stdout);
    process.exit(result.status ?? 1);
  }

  const unexpected = diagnostics.filter(
    (diagnostic) => !isAllowedStorageBreakingDiagnostic(diagnostic),
  );
  if (diagnostics.length === 0 || unexpected.length > 0) {
    process.stdout.write(result.stdout);
    process.exit(result.status ?? 1);
  }

  for (const diagnostic of diagnostics) {
    console.log(`Allowed transient schema cleanup: ${diagnostic.message}`);
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  main();
}
