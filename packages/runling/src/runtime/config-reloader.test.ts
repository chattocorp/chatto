import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, vi } from "vitest";
import { ConfigReloader } from "./config-reloader.ts";

async function fixture(run: (loader: ConfigReloader, path: string) => Promise<void>) {
  const directory = await mkdtemp(join(tmpdir(), "runling-config-test-"));
  const path = join(directory, "runling.config.ts");
  const loader = new ConfigReloader(path);
  try {
    await run(loader, path);
  } finally {
    await loader.close();
    await rm(directory, { recursive: true, force: true });
  }
}

test("starts empty without a config and watches for its creation", async () => {
  await fixture(async (loader, path) => {
    expect(await loader.load()).toEqual({ webhooks: {} });
    expect(loader.error).toBeUndefined();
    expect(await loader.reload()).toEqual({ webhooks: {} });
    expect(loader.error).toBeUndefined();
    const revision = loader.revision;
    await writeFile(path, "export default { webhooks: {} };\n");
    await vi.waitFor(() => expect(loader.revision).toBeGreaterThan(revision), { timeout: 5000 });
    expect(loader.error).toBeUndefined();
  });
});

test("retains a previously loaded config if the file is removed", async () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    await fixture(async (loader, path) => {
      await writeFile(path, "export default { webhooks: {} };\n");
      const config = await loader.load();
      await rm(path);
      expect(await loader.reload()).toBe(config);
      expect(loader.error).toBeDefined();
    });
  } finally {
    log.mockRestore();
  }
});

test("shares the initial load and caches it until reload", async () => {
  await fixture(async (loader, path) => {
    await writeFile(path, "export default { webhooks: {} };\n");
    const [first, second] = await Promise.all([loader.load(), loader.load()]);
    expect(second).toBe(first);
    expect(await loader.load()).toBe(first);
    const next = await loader.reload();
    expect(next).not.toBe(first);
    expect(await loader.load()).toBe(next);
  });
});

test("recovers when an initially invalid config is corrected", async () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    await fixture(async (loader, path) => {
      await writeFile(path, "export default { invalid: true };\n");
      await expect(loader.load()).rejects.toThrow("valid Runling configuration");
      await writeFile(path, "export default { webhooks: {} };\n");
      await vi.waitFor(() => expect(loader.error).toBeUndefined(), { timeout: 5000 });
      expect(await loader.load()).toEqual({ webhooks: {} });
    });
  } finally {
    log.mockRestore();
  }
});

test("rejects an existing invalid config and missing imports", async () => {
  const log = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    await fixture(async (loader, path) => {
      await writeFile(path, "export default { invalid: true };\n");
      await expect(loader.load()).rejects.toThrow("valid Runling configuration");
    });
    await fixture(async (loader, path) => {
      await writeFile(path, "import './missing.ts'; export default { webhooks: {} };\n");
      await expect(loader.load()).rejects.toThrow();
    });
  } finally {
    log.mockRestore();
  }
});

test("agent subpath imports share the host runtime across config reloads", async () => {
  const { agent, connectAgent } = await import("./agents/index.ts");
  const { createWorkflowContext } = await import("./context.ts");

  await fixture(async (loader, path) => {
    await writeFile(path, `
      import { task, Type } from "runling";
      import { agent, connectAgent } from "runling/agents";
      import { startWorkflow } from "runling/web";
      export default { webhooks: {
        probe: startWorkflow(task({ name: "Probe", input: Type.Null(), output: Type.Any() },
          () => ({ agent, connectAgent })))
      }};
    `);
    for (const config of [await loader.load(), await loader.reload()]) {
      let result: unknown;
      await config.webhooks.probe!({
        start: async (task, { input }) => {
          result = await task(createWorkflowContext(), input);
          return { id: "probe" };
        },
      }, null);
      expect(result).toEqual({ agent, connectAgent });
    }
  });
});
