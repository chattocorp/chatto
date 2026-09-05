import { execFileSync } from "node:child_process";
import { appendFileSync, readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const groups = [
  "chatto",
  "workspace",
  "desktop",
  "authling",
  "shared",
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
    } else if (file.startsWith("pkg/"))
      enable("shared", "chatto", "authling", "performance");
    else if (file.startsWith("cli/")) enable("chatto", "performance");
    else if (
      /^(apps\/frontend|packages\/api-types|packages\/lingua)\//.test(file)
    ) {
      enable("workspace", "chatto", "desktop", "performance");
    } else if (file.startsWith("proto/")) {
      enable("chatto", "workspace", "desktop", "performance");
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
  return {
    ...selected,
    e2e: selected.chatto ? "full" : selected.authling ? "integration" : "none",
  };
}

function main() {
  // Selection is a PR optimization only. Pushes, including release branches,
  // must run the complete suite before the existing publication workflows.
  let selected = selectJobs([], true);
  if (process.env.GITHUB_EVENT_NAME === "pull_request") {
    const { pull_request: pr } = JSON.parse(
      readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"),
    );
    const files = execFileSync(
      "git",
      [
        "diff",
        "--name-only",
        "--no-renames",
        "-z",
        `${pr.base.sha}...${pr.head.sha}`,
      ],
      { encoding: "utf8" },
    )
      .split("\0")
      .filter(Boolean);
    selected = selectJobs(files);
  }
  for (const [key, value] of Object.entries(selected))
    appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  main();
