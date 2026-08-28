import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

// This exact file set moved from chatto.core.v1 to lifecycle-specific packages.
// The Go compatibility test validates the stored wire contracts during this
// one-time move. Buf resumes its strict file check after the move is in the base.
const relocatedStorageFiles = new Set([
  "chatto/core/v1/asset_events.proto",
  "chatto/core/v1/auth_events.proto",
  "chatto/core/v1/authorization_events.proto",
  "chatto/core/v1/config_events.proto",
  "chatto/core/v1/credential_usage.proto",
  "chatto/core/v1/encryption_keys.proto",
  "chatto/core/v1/event.proto",
  "chatto/core/v1/invitation_events.proto",
  "chatto/core/v1/message_events.proto",
  "chatto/core/v1/models.proto",
  "chatto/core/v1/moderation_events.proto",
  "chatto/core/v1/notification.proto",
  "chatto/core/v1/oauth_client_events.proto",
  "chatto/core/v1/push.proto",
  "chatto/core/v1/rbac_events.proto",
  "chatto/core/v1/reaction_events.proto",
  "chatto/core/v1/room_events.proto",
  "chatto/core/v1/room_group_events.proto",
  "chatto/core/v1/thread_events.proto",
  "chatto/core/v1/user_events.proto",
  "chatto/core/v1/user_preferences.proto",
]);

export function isExactCorePackageRelocation(diagnostics) {
  if (diagnostics.length !== relocatedStorageFiles.size) return false;

  const deletedFiles = new Set();
  for (const diagnostic of diagnostics) {
    if (diagnostic.type !== "FILE_NO_DELETE") return false;
    const match = diagnostic.message.match(
      /^Previously present file "([^"]+)" was deleted\.$/,
    );
    if (!match || !relocatedStorageFiles.has(match[1])) return false;
    deletedFiles.add(match[1]);
  }
  return deletedFiles.size === relocatedStorageFiles.size;
}

function runCompatibilityTest(repoDir) {
  const result = spawnSync(
    "go",
    ["test", "./internal/protocompat", "-count=1"],
    { cwd: path.join(repoDir, "cli"), encoding: "utf8" },
  );
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

function main() {
  const against = process.argv[2];
  if (!against) {
    console.error("usage: check-storage-proto-breaking.mjs <against-input>");
    process.exit(2);
  }

  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const repoDir = path.resolve(scriptDir, "..");
  const protoDir = path.join(repoDir, "proto");

  runCompatibilityTest(repoDir);

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
      "--exclude-path",
      "chatto/core/live/v1",
      "--exclude-path",
      "chatto/core/projection/v1",
      "--error-format=json",
    ],
    { cwd: protoDir, encoding: "utf8" },
  );

  if (result.error) throw result.error;
  if (result.stderr) process.stderr.write(result.stderr);
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

  if (!isExactCorePackageRelocation(diagnostics)) {
    process.stdout.write(result.stdout);
    process.exit(result.status ?? 1);
  }

  console.log(
    "Allowed the one-time core protobuf package relocation after the 0.4 storage compatibility test passed.",
  );
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  main();
}
