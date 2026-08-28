import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

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
    ],
    { cwd: protoDir, encoding: "utf8" },
  );

  if (result.error) throw result.error;
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.stdout) process.stdout.write(result.stdout);
  process.exit(result.status ?? 1);
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  main();
}
