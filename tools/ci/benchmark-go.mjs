import { spawnSync } from "node:child_process";
import {
  appendFileSync,
  mkdirSync,
  openSync,
  closeSync,
  writeFileSync,
} from "node:fs";
import { performance } from "node:perf_hooks";

mkdirSync(".context/ci", { recursive: true });
const results = [];
// Compile before timing. Disable test-result caching for every measured pass.
const warmup = spawnSync(
  "go",
  [
    "test",
    "-trimpath",
    "-p",
    "1",
    "-tags",
    "test_endpoints",
    "-run",
    "^$",
    "./...",
  ],
  { cwd: "cli", stdio: "inherit" },
);
if (warmup.status !== 0) throw new Error("Go compilation failed");
for (const parallelism of [1, 2, 2, 1]) {
  const log = `.context/ci/go-benchmark-${results.length + 1}-p${parallelism}.log`;
  const fd = openSync(log, "w");
  const start = performance.now();
  const result = spawnSync(
    "go",
    [
      "test",
      "-trimpath",
      "-count=1",
      "-p",
      String(parallelism),
      "-tags",
      "test_endpoints",
      "-timeout",
      "10m",
      "./...",
    ],
    { cwd: "cli", stdio: ["ignore", fd, fd] },
  );
  closeSync(fd);
  results.push({
    parallelism,
    seconds: Math.round((performance.now() - start) / 100) / 10,
    exitCode: result.status,
    log,
  });
  writeFileSync(
    ".context/ci/go-benchmark.json",
    JSON.stringify(results, null, 2),
  );
  console.log(results.at(-1));
}
const summary = `## Go package parallelism\n\nSame runner and revision; warm compilation cache; test-result cache disabled.\n\n| Pass | Packages | Seconds | Exit code |\n| --- | ---: | ---: | ---: |\n${results.map((r, i) => `| ${i + 1} | ${r.parallelism} | ${r.seconds} | ${r.exitCode} |`).join("\n")}\n`;
if (process.env.GITHUB_STEP_SUMMARY)
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, summary);
if (results.some((result) => result.exitCode !== 0)) process.exitCode = 1;
