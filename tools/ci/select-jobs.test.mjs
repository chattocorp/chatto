import assert from "node:assert/strict";
import test from "node:test";
import { selectJobs } from "./select-jobs.mjs";

test("source roots select complete products", () => {
  const cases = [
    ["cli/go.mod", ["chatto"]],
    ["authling/internal/account.go", ["authling"]],
    ["authling/docs/README.md", ["authling"]],
    ["pkg/events/event.go", ["chatto", "authling"]],
    ["apps/frontend/src/app.css", ["chatto", "desktop"]],
    ["packages/lingua/src/index.ts", ["chatto", "desktop"]],
    ["proto/chatto/api/v1/room.proto", ["chatto", "desktop"]],
    ["docker/Dockerfile", ["chatto"]],
    ["apps/desktop/main.mjs", ["desktop"]],
    ["tools/test-desktop-macos-capture.sh", ["desktop"]],
    ["apps/docs-website/src/content/docs/start.md", ["docs"]],
    ["docs/README.md", []],
    ["AGENTS.md", []],
    [".agents/skills/new/SKILL.md", []],
  ];
  for (const [file, expected] of cases) {
    const actual = Object.entries(selectJobs([file]))
      .filter(([, enabled]) => enabled)
      .map(([name]) => name);
    assert.deepEqual(actual.sort(), expected.sort(), file);
  }
});

test("repository tooling and unknown paths select every product", () => {
  for (const file of [
    "mise.toml",
    "go.work",
    "pnpm-lock.yaml",
    ".github/workflows/ci.yml",
    "future-product/main.go",
  ]) {
    assert.deepEqual(selectJobs([file]), {
      chatto: true,
      authling: true,
      docs: true,
      desktop: true,
    });
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
      JSON.stringify({
        pull_request: {
          base: { sha: before },
          head: { sha: git("rev-parse", "HEAD") },
        },
      }),
    );
    execFileSync(process.execPath, [script.pathname], {
      cwd,
      env: {
        ...process.env,
        GITHUB_EVENT_NAME: "pull_request",
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

test("main and release pushes run every group without consulting a file diff", async () => {
  const { execFileSync } = await import("node:child_process");
  const { mkdtempSync, readFileSync, rmSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const cwd = mkdtempSync(join(tmpdir(), "ci-push-"));
  try {
    for (const ref of ["refs/heads/main", "refs/heads/release-0.4"]) {
      const output = join(cwd, ref.split("/").at(-1));
      execFileSync(
        process.execPath,
        [new URL("./select-jobs.mjs", import.meta.url).pathname],
        {
          cwd,
          env: {
            ...process.env,
            GITHUB_EVENT_NAME: "push",
            GITHUB_REF: ref,
            GITHUB_EVENT_PATH: join(cwd, "deliberately-missing-event.json"),
            GITHUB_OUTPUT: output,
          },
        },
      );
      const values = readFileSync(output, "utf8").trim().split("\n");
      assert.equal(values.length, 4);
      for (const value of values) assert.match(value, /=true$/);
    }
  } finally {
    rmSync(cwd, { recursive: true, force: true });
  }
});
