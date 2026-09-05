import { execFileSync } from "node:child_process";
import { appendFileSync, readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const groups = [
  "chatto",
  "workspace",
  "desktop",
  "authling",
  "shared",
  "proto",
  "performance",
  "docs",
];

/** Select consumers of changed files. Unknown paths run all checks. */
export function selectJobs(files, full = false) {
  const selected = Object.fromEntries(groups.map((group) => [group, full]));
  const enable = (...names) =>
    names.forEach((name) => {
      selected[name] = true;
    });
  for (const file of files) {
    if (file.startsWith("apps/docs-website/")) {
      enable("docs");
      continue;
    }
    // Prose and agent instructions do not change either product's runtime.
    if (
      /^(docs|authling\/docs|\.agents|\.claude|\.conductor)\//.test(file) ||
      /(^|\/)(AGENTS|CLAUDE|README)\.md$/.test(file) ||
      /^[^/]+\.md$/.test(file)
    )
      continue;
    // Go workspace dependency selection can affect both products.
    if (/^(cli|authling)\/go\.(mod|sum)$/.test(file))
      enable("shared", "chatto", "authling", "performance");
    else if (/^tools\/.*desktop/.test(file)) enable("desktop");
    else if (file.startsWith("apps/desktop/")) enable("desktop");
    else if (file.startsWith("authling/")) {
      enable("authling");
      if (
        file.endsWith(".proto") ||
        file.endsWith(".pb.go") ||
        /\/buf\./.test(file)
      )
        enable("proto");
    } else if (file.startsWith("pkg/"))
      enable("shared", "chatto", "authling", "performance");
    else if (file.startsWith("cli/")) enable("chatto", "performance");
    else if (
      /^(apps\/frontend|packages\/api-types|packages\/lingua)\//.test(file)
    ) {
      enable("workspace", "chatto", "desktop", "performance");
    } else if (file.startsWith("proto/")) {
      enable("proto", "chatto", "workspace", "desktop", "performance");
    } else if (file.startsWith("docker/")) enable("chatto");
    else if (/^(LICENSES\/|LICENSE$|NOTICE$)/.test(file))
      enable("chatto", "desktop", "docs");
    else if (/^(REUSE.toml$|\.release-please|release-please)/.test(file))
      continue;
    else
      groups.forEach((group) => {
        selected[group] = true;
      });
  }
  // Schema contracts depend on Go types as well as .proto files.
  if (selected.chatto || selected.shared) selected.proto = true;
  return {
    ...selected,
    e2e: selected.chatto ? "full" : selected.authling ? "integration" : "none",
  };
}

function main() {
  const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"));
  const full = ["schedule", "workflow_dispatch"].includes(
    process.env.GITHUB_EVENT_NAME,
  );
  let files = [];
  if (!full) {
    const pr = event.pull_request;
    const base = pr?.base.sha ?? event.before;
    const head = pr?.head.sha ?? event.after;
    if (!base || /^0+$/.test(base)) {
      files = ["unknown-base"];
    } else {
      // A complete checkout permits merge-base comparison for stacked PRs.
      files = execFileSync(
        "git",
        [
          "diff",
          "--name-only",
          "--no-renames",
          "-z",
          pr ? `${base}...${head}` : `${base}..${head}`,
        ],
        { encoding: "utf8" },
      )
        .split("\0")
        .filter(Boolean);
    }
  }
  const mode = event.inputs?.suite ?? "full";
  if (!["full", "performance", "benchmark-go"].includes(mode))
    throw new Error("Unknown CI suite");
  const selected =
    process.env.GITHUB_EVENT_NAME === "workflow_dispatch" && mode !== "full"
      ? { ...selectJobs([]), performance: mode === "performance" }
      : selectJobs(files, full);
  for (const [key, value] of Object.entries(selected))
    appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`);
  appendFileSync(
    process.env.GITHUB_STEP_SUMMARY,
    `## CI selection\n\n${Object.entries(selected)
      .map(([key, value]) => `- ${key}: ${value}`)
      .join("\n")}\n`,
  );
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  main();
