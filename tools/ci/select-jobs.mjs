import { execFileSync } from "node:child_process";
import { appendFileSync, readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

// Explicit product boundaries. Repository-wide and unknown paths select all.
const products = {
  chatto: ["cli/", "pkg/", "apps/frontend/", "packages/", "proto/", "docker/"],
  authling: ["authling/", "pkg/"],
  docs: ["apps/docs-website/"],
  desktop: [
    "apps/desktop/",
    "apps/frontend/",
    "packages/",
    "proto/",
    "tools/test-desktop-",
  ],
};

/** Select complete product checks from their source roots. */
export function selectJobs(files, full = false) {
  const selected = Object.fromEntries(
    Object.keys(products).map((name) => [name, full]),
  );
  for (const file of files) {
    const matches = Object.entries(products).filter(([, roots]) =>
      roots.some((root) => file.startsWith(root)),
    );
    if (matches.length) {
      for (const [name] of matches) selected[name] = true;
    } else if (
      !/^(docs\/|\.agents\/|\.claude\/|\.conductor\/|[^/]+\.md$)/.test(file)
    ) {
      for (const name of Object.keys(products)) selected[name] = true;
    }
  }
  return selected;
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
