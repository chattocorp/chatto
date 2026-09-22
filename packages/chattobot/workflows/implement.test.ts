import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, readFile, writeFile, rm, readdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import type { AgentExtensionAPI, AgentOptions, AgentRunOptions } from "runling/agents";
import { createAgentTasks } from "runling/agents";
import { createImplementation, implementationExtension, implementationSettings, matchesRepository } from "./implement.ts";
import { implementationProcess, ImplementationCommandError, type ImplementationProcess } from "./implementation-process.ts";

const exec = promisify(execFile);
const folders: string[] = [];
afterEach(async () => {
  vi.unstubAllEnvs();
  await Promise.all(folders.splice(0).map(folder => rm(folder, { recursive: true, force: true })));
});
async function fixture() {
  const folder = await mkdtemp(join(tmpdir(), "chattobot-implement-"));
  folders.push(folder);
  const directory = join(folder, "repo");
  const remote = join(folder, "remote.git");
  await exec("git", ["init", "--initial-branch=main", directory]);
  const git = async (...args: string[]) => (await exec("git", args, { cwd: directory })).stdout.trim();
  await git("config", "user.name", "Test");
  await git("config", "user.email", "test@example.invalid");
  await writeFile(join(directory, "example.txt"), "original\n");
  await git("add", ".");
  await git("-c", "commit.gpgsign=false", "commit", "-m", "fixture");
  await exec("git", ["init", "--bare", remote]);
  await git("remote", "add", "origin", remote);
  await git("push", "origin", "main");
  const settings = { directory, repository: "example/chatto", artifactsDirectory: join(folder, "artifacts") };
  const calls: { command: string; args: string[] }[] = [];
  let published = false;
  let body = "";
  let branch = "";
  const execute: ImplementationProcess = async (command, args, options) => {
    calls.push({ command, args });
    if (command === "mise") {
      if (args.includes("install")) return "";
      return implementationProcess("bash", ["-c", check], options);
    }
    if (command === "git" && args.includes("get-url")) return "git@github.com:example/chatto.git\n";
    if (command === "gh") {
      if (args[0] === "auth") return "";
      if (args[1] === "create") {
        branch = args[args.indexOf("--head") + 1]!;
        body = await readFile(args[args.indexOf("--body-file") + 1]!, "utf8");
        published = true;
        return "https://github.com/example/chatto/pull/7\n";
      }
      if (!published) throw new Error("No PR");
      return JSON.stringify({ url: "https://github.com/example/chatto/pull/7", headRefName: branch, headRefOid: await git("rev-parse", branch), baseRefName: "main", state: "OPEN" });
    }
    return implementationProcess(command, args, options);
  };
  return { settings, execute, git, remote, calls, get body() { return body; } };
}

type Tool = { name: string; execute: (id: string, input: never, signal?: AbortSignal) => Promise<{ isError?: boolean; content: { text?: string }[] }> };
async function workerTools(options: AgentOptions) {
  const tools = new Map<string, Tool>();
  for (const extension of options.extensions ?? []) {
    const factory = typeof extension === "function" ? extension : extension.factory;
    await factory({ registerTool(tool: Tool) { tools.set(tool.name, tool); } } as unknown as AgentExtensionAPI);
  }
  return async (name: string, input: object) => tools.get(name)!.execute("call", input as never);
}
const patch = "diff --git a/example.txt b/example.txt\n--- a/example.txt\n+++ b/example.txt\n@@ -1 +1 @@\n-original\n+fixed\n";
const check = "test \"$(cat example.txt)\" = fixed";
const proposal = { title: "fix(example): correct the value", summary: "Correct the value to fix the reported behavior.", notes: ["Browser behavior was not checked."] };
function worker(action: (options: AgentOptions, call: Awaited<ReturnType<typeof workerTools>>) => Promise<void>, outcome: "completed" | "blocked" = "completed") {
  return async (options: AgentOptions) => ({
    dispose: vi.fn(),
    async runOutcome(_ctx: unknown, _prompt: string, runOptions?: AgentRunOptions) {
      runOptions?.onText?.("Worker commentary for the supervisor");
      await action(options, await workerTools(options));
      return { outcome, summary: "Model-authored URL must not be trusted: https://example.invalid/pr", usage: emptyTokenUsage() };
    },
  });
}

test("implements in an isolated worktree, records final checks, pushes and verifies a ready PR", async () => {
  const f = await fixture();
  await writeFile(join(f.settings.directory, "example.txt"), "user dirty change\n");
  const originalBranch = await f.git("branch", "--show-current");
  const updates: unknown[] = [];
  const implement = createImplementation(f.settings, { execute: f.execute, createAgent: worker(async (options, call) => {
    expect(options.tools).toContain("apply_patch");
    expect(options.tools).not.toContain("write");
    expect(options.tools).not.toContain("bash");
    expect(options.tools).not.toContain("runCheck");
    expect(f.calls.some(call => call.args.includes("install"))).toBe(true);
    expect(await readFile(join(options.cwd, "example.txt"), "utf8")).toBe("original\n");
    await call("apply_patch", { patch });
    await call("preparePullRequest", proposal);
  }) });
  const result = await implement({ ...createWorkflowContext(), emit: async value => { updates.push(value); } }, { request: "Fix the value" });
  expect(result).toMatchObject({ outcome: "completed", prUrl: "https://github.com/example/chatto/pull/7", summary: proposal.summary, notes: proposal.notes, checks: [{ command: "mise x -- pnpm run check", passed: true }, { command: "mise x -- pnpm run test", passed: true }] });
  expect(f.body).toContain("## Verification");
  expect(f.body).toContain("mise x -- pnpm run test");
  expect(f.calls.find(call => call.args[1] === "create")?.args).not.toContain("--draft");
  expect(await f.git("branch", "--show-current")).toBe(originalBranch);
  expect(await readFile(join(f.settings.directory, "example.txt"), "utf8")).toBe("user dirty change\n");
  expect((await exec("git", ["--git-dir", f.remote, "show", `${result.branch}:example.txt`])).stdout).toBe("fixed\n");
  expect(updates).toContainEqual(expect.objectContaining({ type: "finding", text: expect.stringContaining("validation passed") }));
  expect(updates).toContainEqual({ type: "output", text: "Worker commentary for the supervisor" });
  expect(updates).toContainEqual({ type: "state", value: { phase: "validating", currentCheck: "mise x -- pnpm run test",
    completedChecks: ["mise x -- pnpm run check"], pendingChecks: ["mise x -- pnpm run test"] } });
  expect(updates).toContainEqual({ type: "state", value: { phase: "published", prUrl: result.prUrl,
    completedChecks: ["mise x -- pnpm run check", "mise x -- pnpm run test"], pendingChecks: [] } });
  const [folder] = await readdir(f.settings.artifactsDirectory);
  expect(JSON.parse(await readFile(join(f.settings.artifactsDirectory, folder!, "metadata.json"), "utf8"))).toMatchObject({ stage: "published", prUrl: result.prUrl });
});

test.each(["protected-file", "empty", "blocked", "missing-proposal", "history-changed", "branch-changed"])("does not publish %s", async mode => {
  const f = await fixture();
  const result = await createImplementation(f.settings, { execute: f.execute, createAgent: worker(async (options, call) => {
    if (mode !== "empty") await call("apply_patch", { patch });
    if (mode === "protected-file") await writeFile(join(options.cwd, "AGENTS.md"), "changed instructions");
    if (mode !== "missing-proposal") await call("preparePullRequest", proposal);
    if (mode === "history-changed") await exec("git", ["-c", "commit.gpgsign=false", "commit", "-am", "worker commit"], { cwd: options.cwd });
    if (mode === "branch-changed") await exec("git", ["checkout", "-b", "unexpected-branch"], { cwd: options.cwd });
  }, mode === "blocked" ? "blocked" : "completed") })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe("blocked");
  expect(result.prUrl).toBeUndefined();
  expect(f.calls.some(call => call.args.includes("push") || call.args[1] === "create")).toBe(false);
});

test("host validation returns diagnostics to the same worker and checks the repaired final tree", async () => {
  const f = await fixture();
  let turns = 0;
  let validations = 0;
  const createAgent = vi.fn(async (options: AgentOptions) => ({ dispose: vi.fn(), async runOutcome(_ctx: unknown, prompt: string) {
    const call = await workerTools(options);
    if (++turns === 1) await call("apply_patch", { patch });
    else {
      expect(prompt).toContain("Expected regression coverage");
      await call("apply_patch", { patch: "--- /dev/null\n+++ b/regression.txt\n@@ -0,0 +1 @@\n+covered\n" });
    }
    await call("preparePullRequest", proposal);
    return { outcome: "completed" as const, summary: "Ready", usage: emptyTokenUsage() };
  } }));
  const result = await createImplementation(f.settings, { createAgent, execute: async (command, args, options) => {
    if (command === "mise" && args.at(-1) === "check" && ++validations === 1) throw new ImplementationCommandError("Check failed", "Expected regression coverage");
    return f.execute(command, args, options);
  } })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe("completed");
  expect(createAgent).toHaveBeenCalledOnce();
  expect(turns).toBe(2);
  expect(result.checks.every(check => check.passed)).toBe(true);
});

test("patch errors give the worker Git diagnostics and incorrect hunk counts can be repaired", async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, { execute: f.execute, createAgent: worker(async (_options, call) => {
    const bad = await call("apply_patch", { patch: patch.replace("-original", "-not present") });
    expect(bad.isError).toBe(true);
    expect(bad.content[0]?.text).toContain("patch does not apply");
    const fixed = await call("apply_patch", { patch: patch.replace("@@ -1 +1 @@", "@@ -1,8 +1,20 @@") });
    expect(fixed.isError).toBeUndefined();
    await call("preparePullRequest", proposal);
  }) })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe("completed");
});

test.each([
  { path: "apps/frontend/example.txt", scripts: ["check:frontend", "test:frontend"] },
  { path: "cli/example.go", scripts: ["check", "test", "test-cli"] },
])("host selects validation for $path", async ({ path, scripts }) => {
  const f = await fixture();
  const validation: string[] = [];
  const result = await createImplementation(f.settings, { execute: async (command, args, options) => {
    if (command === "mise" && !args.includes("install")) { validation.push(args.at(-1)!); return ""; }
    return f.execute(command, args, options);
  }, createAgent: worker(async (_options, call) => {
    await call("apply_patch", { patch: `--- /dev/null\n+++ b/${path}\n@@ -0,0 +1 @@\n+fixture\n` });
    await call("preparePullRequest", proposal);
  }) })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe("completed");
  expect(validation).toEqual(scripts);
});

test.each(["failed-check", "source-changing-check"])("bounds repairs for %s and never publishes", async mode => {
  const f = await fixture();
  let turns = 0;
  let checks = 0;
  const createAgent = vi.fn(worker(async (_options, call) => {
    if (++turns === 1) await call("apply_patch", { patch });
    await call("preparePullRequest", proposal);
  }));
  const result = await createImplementation(f.settings, { createAgent, execute: async (command, args, options) => {
    if (command === "mise" && args.at(-1) === "check") {
      if (mode === "failed-check") throw new ImplementationCommandError("Check failed", "Assertion failed");
      await writeFile(join(options.cwd, "generated.txt"), String(++checks));
    }
    return f.execute(command, args, options);
  } })(createWorkflowContext(), { request: "Fix" });
  expect(result).toMatchObject({ outcome: "blocked", summary: expect.stringContaining("three attempts") });
  expect(turns).toBe(3);
  expect(createAgent).toHaveBeenCalledOnce();
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
});

test.each(["failed", "changed-source"])("setup %s stops before worker creation", async mode => {
  const f = await fixture();
  const createAgent = vi.fn();
  const result = await createImplementation(f.settings, { createAgent, execute: async (command, args, options) => {
    if (command === "mise" && args.includes("install")) {
      if (mode === "failed") throw new Error("Private setup output");
      await writeFile(join(options.cwd, "example.txt"), "unexpected setup change");
    }
    return f.execute(command, args, options);
  } })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe("blocked");
  expect(JSON.stringify(result)).not.toContain("Private setup output");
  expect(createAgent).not.toHaveBeenCalled();
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
});

test("cancellation during host validation disposes the same worker and prevents publication", async () => {
  const f = await fixture();
  const controller = new AbortController();
  const dispose = vi.fn();
  await expect(createImplementation(f.settings, { createAgent: async options => ({ dispose, async runOutcome() {
    const call = await workerTools(options);
    await call("apply_patch", { patch }); await call("preparePullRequest", proposal);
    return { outcome: "completed", summary: "Ready", usage: emptyTokenUsage() };
  } }), execute: async (command, args, options) => {
    if (command === "mise" && args.at(-1) === "check") { controller.abort(new Error("Cancelled check")); options.signal.throwIfAborted(); }
    return f.execute(command, args, options);
  } })({ ...createWorkflowContext(), signal: controller.signal }, { request: "Fix" })).rejects.toThrow("Cancelled check");
  expect(dispose).toHaveBeenCalledOnce();
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
});

test("notifications cannot restart a failed implementation; a new human request can", async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  let version: number | undefined = 1;
  const announce = vi.fn(async () => {});
  const extension = implementationExtension(ctx, f.settings, announce, tasks, { requestVersion: () => version,
    execute: f.execute, createAgent: worker(async () => {}, "blocked"),
  });
  const call = await workerTools({ cwd: f.settings.directory, model: "test/model", extensions: [extension] });
  const input = { request: "Fix", announcement: "I'll implement this." };
  try {
    await call("implementChatto", input);
    const duplicate = await call("implementChatto", input);
    expect(JSON.parse(duplicate.content[0]!.text!).outcome).toBe("blocked");
    await vi.waitFor(() => expect(tasks.list().some(task => task.status === "running")).toBe(false));
    expect(JSON.parse((await call("implementChatto", input)).content[0]!.text!).outcome).toBe("blocked");
    version = undefined;
    expect(JSON.parse((await call("implementChatto", input)).content[0]!.text!).outcome).toBe("blocked");
    expect(announce).toHaveBeenCalledOnce();
    version = 2;
    expect(JSON.parse((await call("implementChatto", input)).content[0]!.text!).status).toBe("running");
    await vi.waitFor(() => expect(tasks.list().some(task => task.status === "running")).toBe(false));
    expect(tasks.list()).toHaveLength(2);
  } finally { await tasks.dispose(); }
});

test.each(["response-lost", "not-created", "wrong-url", "wrong-commit"])("handles publication uncertainty: %s", async mode => {
  const f = await fixture();
  const execute: ImplementationProcess = async (command, args, options) => {
    if (command === "gh" && args[1] === "create") {
      if (mode === "response-lost") await f.execute(command, args, options);
      throw new Error("Publication response lost");
    }
    if (command === "gh" && args[1] === "view" && mode === "wrong-url") return JSON.stringify({ url: "https://example.invalid/pull/7", headRefName: args[2], baseRefName: "main", state: "OPEN" });
    if (command === "gh" && args[1] === "view" && mode === "wrong-commit") return JSON.stringify({ url: "https://github.com/example/chatto/pull/7", headRefName: args[2], headRefOid: "wrong", baseRefName: "main", state: "OPEN" });
    return f.execute(command, args, options);
  };
  const result = await createImplementation(f.settings, { execute, createAgent: worker(async (_options, call) => {
    await call("apply_patch", { patch }); await call("preparePullRequest", proposal);
  }) })(createWorkflowContext(), { request: "Fix" });
  expect(result.outcome).toBe(mode === "response-lost" ? "completed" : "publication_unknown");
  expect(result.prUrl).toBe(mode === "response-lost" ? "https://github.com/example/chatto/pull/7" : undefined);
});

test("cancellation stops work, disposes the worker and retains the patch without publication", async () => {
  const f = await fixture();
  const controller = new AbortController();
  const dispose = vi.fn();
  await expect(createImplementation(f.settings, { execute: f.execute, createAgent: async options => ({ dispose, async runOutcome() {
    await (await workerTools(options))("apply_patch", { patch });
    controller.abort();
    return { outcome: "completed", summary: "Cancelled", usage: emptyTokenUsage() };
  } }) })({ ...createWorkflowContext(), signal: controller.signal }, { request: "Fix" })).rejects.toThrow();
  expect(dispose).toHaveBeenCalledOnce();
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
  const [folder] = await readdir(f.settings.artifactsDirectory);
  expect(await readFile(join(f.settings.artifactsDirectory, folder!, "changes.patch"), "utf8")).toContain("+fixed");
});

test("settings are opt-in and remote matching cannot select a different host or repository", () => {
  vi.stubEnv("CHATTO_IMPLEMENTATION_REPOSITORY", "");
  expect(implementationSettings()).toBeUndefined();
  vi.stubEnv("CHATTO_IMPLEMENTATION_REPOSITORY", "example/chatto");
  vi.stubEnv("CHATTO_SOURCE_DIRECTORY", "");
  expect(() => implementationSettings()).toThrow("CHATTO_SOURCE_DIRECTORY");
  vi.stubEnv("CHATTO_SOURCE_DIRECTORY", "/repo");
  vi.stubEnv("CHATTO_SOURCE_REF", undefined);
  expect(implementationSettings()).toMatchObject({ directory: "/repo", repository: "example/chatto", baseBranch: "main" });
  vi.stubEnv("CHATTO_SOURCE_REF", "origin/develop");
  expect(implementationSettings()?.baseBranch).toBe("origin/develop");
  expect(matchesRepository("git@github.com:example/chatto.git", "example/chatto")).toBe(true);
  for (const remote of ["https://github.com/other/chatto", "https://github.com.evil/example/chatto", "https://token@github.com/example/chatto"]) expect(matchesRepository(remote, "example/chatto")).toBe(false);
  expect(() => createImplementation({ directory: "/missing", repository: "--help" })).toThrow("owner/repo");
});

test("provider login failure reaches the owner without claiming edits or publication", async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, { execute: f.execute, createAgent: async () => ({
    dispose() {},
    async runOutcome() { return { outcome: "failed", failureReason: "provider_error", summary: "The model provider login has expired. Sign in again on the agent host before retrying.", usage: emptyTokenUsage() }; },
  }) })(createWorkflowContext(), { request: "Fix" });
  expect(result).toMatchObject({ outcome: "blocked", summary: expect.stringContaining("login has expired"), checks: [] });
  expect(result.prUrl).toBeUndefined();
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
});

test("unconsumed steering prevents publication", async () => {
  const f = await fixture();
  const result = await createImplementation(f.settings, { execute: f.execute, createAgent: worker(async (_options, call) => {
    await call("apply_patch", { patch }); await call("preparePullRequest", proposal);
  }) })({ ...createWorkflowContext(), inbox: (async function* () { yield "Do not publish yet"; })() }, { request: "Fix" });
  expect(result).toMatchObject({ outcome: "blocked", summary: expect.stringContaining("clarification was not consumed") });
  expect(f.calls.some(call => call.args.includes("push"))).toBe(false);
});

test("a wrong origin fails before worker creation or any fetch", async () => {
  const f = await fixture();
  const createAgent = vi.fn();
  const result = await createImplementation(f.settings, { createAgent, execute: async (command, args, options) => {
    if (args.includes("get-url")) return "https://github.com/other/repo.git";
    return f.execute(command, args, options);
  } })(createWorkflowContext(), { request: "Fix" });
  expect(result).toMatchObject({ outcome: "blocked", summary: expect.stringContaining("origin matches") });
  expect(createAgent).not.toHaveBeenCalled();
  expect(f.calls.some(call => call.args.includes("fetch"))).toBe(false);
});

test.each(["origin/main", "refs/remotes/origin/main"])("accepts remote-tracking base %s", async baseBranch => {
  const f = await fixture();
  const createAgent = vi.fn(worker(async () => {}, "blocked"));
  await createImplementation({ ...f.settings, baseBranch }, { execute: f.execute, createAgent })(createWorkflowContext(), { request: "Fix" });
  expect(createAgent).toHaveBeenCalledOnce();
  expect(f.calls.find(call => call.args.includes("fetch"))?.args).toContain("refs/heads/main:refs/remotes/origin/main");
});

test.each(["auth", "fetch"])("reports a safe %s preflight failure without starting the worker", async stage => {
  const f = await fixture();
  const createAgent = vi.fn();
  const result = await createImplementation(f.settings, { createAgent, execute: async (command, args, options) => {
    if (args.includes(stage)) throw new Error("private credentials and subprocess output");
    return f.execute(command, args, options);
  } })(createWorkflowContext(), { request: "Fix" });
  expect(result).toMatchObject({ outcome: "blocked", summary: expect.stringContaining(stage === "auth" ? "authentication check" : "base branch"), checks: [] });
  expect(JSON.stringify(result)).not.toContain("private credentials");
  expect(createAgent).not.toHaveBeenCalled();
});

test("the tool waits for its announcement, returns a handle, accepts steering, and reports the PR", async () => {
  const f = await fixture();
  const ctx = createWorkflowContext();
  const tasks = createAgentTasks(ctx);
  const notices = tasks.notifications[Symbol.asyncIterator]();
  const delivered = Promise.withResolvers<void>();
  const started = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const announce = vi.fn(async () => delivered.promise);
  const steer = vi.fn(async () => true);
  const createAgent = vi.fn(async (options: AgentOptions) => ({ steer, dispose: () => {}, async runOutcome() {
    started.resolve();
    await finish.promise;
    const call = await workerTools(options);
    await call("apply_patch", { patch }); await call("preparePullRequest", proposal);
    return { outcome: "completed" as const, summary: "Done", usage: emptyTokenUsage() };
  } }));
  const extension = implementationExtension(ctx, f.settings, announce, tasks, { execute: f.execute, createAgent });
  const call = await workerTools({ cwd: f.settings.directory, model: "test/model", extensions: [extension] });
  try {
    const pending = call("implementChatto", { request: "Fix the value", announcement: "I'll implement the fix and run its checks." });
    await vi.waitFor(() => expect(announce).toHaveBeenCalledOnce());
    expect(createAgent).not.toHaveBeenCalled();
    expect(f.calls).toEqual([]);
    delivered.resolve();
    const handle = JSON.parse((await pending).content[0]!.text!);
    expect(handle.status).toBe("running");
    await started.promise;
    await tasks.send(handle.id, "Keep the public API unchanged");
    await vi.waitFor(() => expect(steer).toHaveBeenCalledWith("Keep the public API unchanged"));
    finish.resolve();
    const notification = JSON.parse((await notices.next()).value!);
    expect(notification.type).toBe("task.completed");
    expect(JSON.parse(notification.task.result)).toMatchObject({ outcome: "completed", prUrl: "https://github.com/example/chatto/pull/7" });
    expect(announce).toHaveBeenCalledOnce();
  } finally { delivered.resolve(); finish.resolve(); await tasks.dispose(); }
});

test.skipIf(!process.env.CHATTO_EVAL_MODEL)("live worker edits a fixture and passes real host setup and validation", async () => {
  const f = await fixture();
  await writeFile(join(f.settings.directory, ".gitignore"), "node_modules/\n");
  await writeFile(join(f.settings.directory, ".tool-versions"), "node 24.21.0\npnpm 11.25.0\n");
  await writeFile(join(f.settings.directory, "package.json"), JSON.stringify({ name: "implementation-fixture", private: true,
    scripts: { check: "node --check regression.cjs", test: "node --test regression.cjs" },
  }));
  await writeFile(join(f.settings.directory, "regression.cjs"), "const {test}=require('node:test'); const {strictEqual}=require('node:assert'); const {readFileSync}=require('node:fs'); test('fixed value',()=>strictEqual(readFileSync('example.txt','utf8'),'fixed\\n'));\n");
  await implementationProcess("mise", ["x", "--", "pnpm", "install", "--lockfile-only", "--ignore-scripts"], { cwd: f.settings.directory, signal: AbortSignal.timeout(30_000), captureDiagnostics: true })
    .catch(error => { throw new Error(error instanceof ImplementationCommandError ? error.output : "Fixture setup failed"); });
  await f.git("add", ".");
  await f.git("-c", "commit.gpgsign=false", "commit", "-m", "test: add fixture validation");
  await f.git("push", "origin", "main");
  const result = await createImplementation({ ...f.settings, model: process.env.CHATTO_EVAL_MODEL, timeoutMs: 120_000 }, {
    execute: (command, args, options) => command === "mise" ? implementationProcess(command, args, options) : f.execute(command, args, options),
  })(createWorkflowContext(), { request: "Fix example.txt: its complete contents must be fixed followed by one newline. The existing regression test defines the correct behavior; do not weaken it. Prepare the PR when the edit is ready. Host validation runs after your report." });
  expect(result.outcome).toBe("completed");
  expect(result.checks).toHaveLength(2);
  expect(result.checks.every(check => check.passed)).toBe(true);
  expect(result.prUrl).toBe("https://github.com/example/chatto/pull/7");
  expect(await readFile(join(f.settings.directory, "example.txt"), "utf8")).toBe("original\n");
}, 160_000);
