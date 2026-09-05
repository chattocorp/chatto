import assert from "node:assert/strict";
import test from "node:test";
import { selectJobs } from "./select-jobs.mjs";

test("documentation does not select runtime checks", () => {
  const result = selectJobs([
    "docs/README.md",
    "authling/AGENTS.md",
    ".agents/skills/new/SKILL.md",
  ]);
  assert.equal(result.e2e, "none");
  assert.equal(result.desktop, false);
  assert.equal(result.performance, false);
});
test("Authling changes retain cross-product login coverage", () => {
  const result = selectJobs(["authling/internal/core/account.go"]);
  assert.equal(result.authling, true);
  assert.equal(result.e2e, "integration");
  assert.equal(result.desktop, false);
  assert.equal(result.performance, false);
});
test("shared modules select both products", () => {
  const result = selectJobs(["pkg/events/event.go"]);
  for (const group of ["chatto", "authling", "shared", "proto", "performance"])
    assert.equal(result[group], true);
});
test("frontend and public protocols also select desktop consumers", () => {
  for (const file of [
    "apps/frontend/src/app.css",
    "proto/chatto/api/v1/room.proto",
    "packages/lingua/src/index.ts",
  ]) {
    const result = selectJobs([file]);
    for (const group of ["chatto", "workspace", "desktop", "performance"])
      assert.equal(result[group], true);
  }
});
test("unknown paths and root dependencies fail open to full coverage", () => {
  for (const file of [
    "future-product/main.go",
    "pnpm-lock.yaml",
    "mise.toml",
    ".github/workflows/ci.yml",
  ]) {
    assert.equal(selectJobs([file]).shared, true);
    assert.equal(selectJobs([file]).docs, true);
  }
});
test("scheduled and manual runs cover everything even without a diff", () => {
  for (const value of Object.values(selectJobs([], true)))
    assert.ok(value === true || value === "full");
});
test("docs website Markdown selects a production docs build", () => {
  const result = selectJobs(["apps/docs-website/src/content/docs/start.md"]);
  assert.equal(result.docs, true);
  assert.equal(result.e2e, "none");
});
test("Markdown within application source is not assumed to be documentation", () => {
  assert.equal(selectJobs(["apps/frontend/src/help.md"]).chatto, true);
});
test("packaging changes do not select a performance benchmark", () => {
  for (const file of [
    "apps/desktop/scripts/build.mjs",
    "tools/test-desktop-macos-capture.sh",
  ]) {
    const result = selectJobs([file]);
    assert.equal(result.desktop, true);
    assert.equal(result.performance, false);
  }
});

test("Git selection includes both sides of a move across products", async () => {
  const { execFileSync } = await import("node:child_process");
  const { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync } =
    await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const script = new URL("./select-jobs.mjs", import.meta.url);
  const cwd = mkdtempSync(join(tmpdir(), "ci-selection-"));
  const git = (...args) =>
    execFileSync("git", args, { cwd, encoding: "utf8" }).trim();
  try {
    git("init", "--quiet");
    git("config", "user.name", "CI test");
    git("config", "user.email", "ci@example.invalid");
    mkdirSync(join(cwd, "cli"));
    writeFileSync(join(cwd, "cli/example.go"), "package example\n");
    git("add", ".");
    git("commit", "--quiet", "-m", "fixture");
    const before = git("rev-parse", "HEAD");
    mkdirSync(join(cwd, "authling"));
    git("mv", "cli/example.go", "authling/example.go");
    git("commit", "--quiet", "-m", "move");
    const event = join(cwd, "event.json");
    const output = join(cwd, "output");
    writeFileSync(
      event,
      JSON.stringify({ before, after: git("rev-parse", "HEAD") }),
    );
    execFileSync(process.execPath, [script.pathname], {
      cwd,
      env: {
        ...process.env,
        GITHUB_EVENT_NAME: "push",
        GITHUB_EVENT_PATH: event,
        GITHUB_OUTPUT: output,
        GITHUB_STEP_SUMMARY: join(cwd, "summary"),
      },
    });
    const selected = readFileSync(output, "utf8");
    assert.match(selected, /^chatto=true$/m);
    assert.match(selected, /^authling=true$/m);
  } finally {
    rmSync(cwd, { recursive: true, force: true });
  }
});

test("either product dependency graph can affect the whole Go workspace", () => {
  for (const file of [
    "cli/go.mod",
    "cli/go.sum",
    "authling/go.mod",
    "authling/go.sum",
  ]) {
    const result = selectJobs([file]);
    for (const group of [
      "chatto",
      "authling",
      "shared",
      "proto",
      "performance",
    ])
      assert.equal(result[group], true);
  }
});
test("hand edits to Authling generated bindings still run the drift check", () => {
  assert.equal(selectJobs(["authling/internal/proto/event.pb.go"]).proto, true);
});
