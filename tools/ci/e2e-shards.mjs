import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

/** Extract the live inventory, including new tests that have no duration hint. */
export function inventory(report) {
  const files = new Map();
  function visit(suite) {
    for (const spec of suite.specs ?? []) {
      const file = spec.file.replace(/^e2e\//, "");
      files.set(file, (files.get(file) ?? 0) + spec.tests.length);
    }
    for (const child of suite.suites ?? []) visit(child);
  }
  visit(report);
  return [...files].map(([file, count]) => ({ file, count }));
}

/** Assign complete files to the lightest shard, longest estimated work first. */
export function balance(files, hints, total) {
  if (!Number.isInteger(total) || total < 1)
    throw new Error("Shard count must be a positive integer");
  const values = Object.values(hints).sort((a, b) => a - b);
  if (values.some((value) => !Number.isFinite(value) || value <= 0))
    throw new Error("Invalid duration hint");
  const fallback = values[Math.floor(values.length / 2)] ?? 3000;
  const shards = Array.from({ length: total }, () => ({
    files: [],
    durationMs: 0,
  }));
  const weighted = files.map(({ file, count }) => ({
    file,
    durationMs: count * (hints[file] ?? fallback),
  }));
  weighted.sort(
    (a, b) => b.durationMs - a.durationMs || a.file.localeCompare(b.file, "en"),
  );
  for (const entry of weighted) {
    const shard = shards.reduce((best, item) =>
      item.durationMs < best.durationMs ? item : best,
    );
    shard.files.push(entry.file);
    shard.durationMs += entry.durationMs;
  }
  return shards;
}

function main() {
  const [current, total] = (process.argv[2] ?? "").split("/").map(Number);
  if (!Number.isInteger(current) || current < 1 || current > total)
    throw new Error("Use CURRENT/TOTAL");
  const root = path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    "../..",
  );
  const cwd = path.join(root, "apps/frontend");
  const hints = JSON.parse(
    readFileSync(path.join(root, "tools/ci/e2e-duration-hints.json"), "utf8"),
  );
  const args = ["exec", "playwright", "test", "--grep-invert", "@ffmpeg"];
  // Listing does not run global setup or start test processes.
  const report = JSON.parse(
    execFileSync("pnpm", [...args, "--list", "--reporter=json"], {
      cwd,
      encoding: "utf8",
      maxBuffer: 16 * 1024 * 1024,
    }),
  );
  if (report.errors?.length)
    throw new Error("Playwright could not collect all tests");
  const shards = balance(inventory(report), hints, total);
  if (!shards.some((shard) => shard.files.length))
    throw new Error("No E2E tests were collected");
  mkdirSync(path.join(root, ".context/ci"), { recursive: true });
  writeFileSync(
    path.join(root, `.context/ci/e2e-shards-${current}.json`),
    JSON.stringify(shards, null, 2),
  );
  console.log(
    shards
      .map(
        (shard, index) =>
          `Shard ${index + 1}: ${shard.files.length} files, ${Math.round(shard.durationMs / 1000)} estimated test-seconds`,
      )
      .join("\n"),
  );
  if (process.argv.includes("--list")) return;
  const selected = shards[current - 1];
  if (!selected.files.length) return;
  // File arguments are regular expressions. Anchor and escape them so similarly
  // named files cannot run in two shards. Serial suites stay in one process.
  const filters = selected.files.map(
    (file) => `/${file.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`,
  );
  const result = spawnSync("pnpm", [...args, ...filters], {
    cwd,
    stdio: "inherit",
    env: process.env,
  });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  main();
