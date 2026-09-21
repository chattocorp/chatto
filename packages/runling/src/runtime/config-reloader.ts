import { serverLog } from "./server-log.ts";
import { dirname, relative, sep } from "node:path";
import { stat } from "node:fs/promises";
import { watch } from "chokidar";
import { createJiti } from "jiti";
import * as runling from "./index.ts";
import * as agents from "./agents/index.ts";
import * as web from "./web-config.ts";
import { isWebConfig, type WebConfig } from "./web-config.ts";

/** Reload project modules without replacing active runs or installed packages. */
export class ConfigReloader {
  private current?: WebConfig;
  private hasLoadedFile = false;
  private pending?: Promise<WebConfig>;
  private timer?: ReturnType<typeof setTimeout>;
  private disposed = false;
  private listeners = new Set<() => void>();
  private watcher;
  private ready: Promise<void>;
  error: string | undefined;
  revision = 0;

  constructor(readonly path: string) {
    const root = dirname(path);
    this.watcher = watch(root, {
      ignoreInitial: true,
      persistent: false,
      ignored: (file, stats) => {
        if (file === path) return false;
        const parts = relative(root, file).split(sep);
        if (
          parts.some(
            (part) =>
              part.startsWith(".") ||
              ["node_modules", "dist", "build", "coverage"].includes(part),
          )
        )
          return true;
        return stats?.isFile() ? !/\.(?:[cm]?[jt]sx?|json)$/.test(file) : false;
      },
    });
    this.ready = new Promise((resolve, reject) => {
      this.watcher.once("ready", resolve);
      this.watcher.on("error", (error) => {
        reject(error);
        this.error = `Configuration watcher failed: ${error instanceof Error ? error.message : String(error)}`;
        serverLog("error", "config.watch_failed", { config: this.path, message: this.error });
        for (const listener of this.listeners) listener();
      });
    });
    this.watcher.on("all", () => {
      clearTimeout(this.timer);
      this.timer = setTimeout(() => {
        void (this.pending ?? Promise.resolve())
          .catch(() => {})
          .then(() => (this.disposed ? undefined : this.reload()))
          .catch(() => {});
      }, 100);
      this.timer.unref();
    });
  }
  load(): Promise<WebConfig> {
    return this.current
      ? Promise.resolve(this.current)
      : (this.pending ?? this.reload());
  }
  reload(): Promise<WebConfig> {
    if (this.pending) return this.pending;
    this.pending = this.read().finally(() => {
      this.pending = undefined;
    });
    return this.pending;
  }
  private async read(): Promise<WebConfig> {
    try {
      await this.ready;
      let missing = false;
      try {
        await stat(this.path);
      } catch (error) {
        // Only an absent entry file is optional, not failed imports or invalid config.
        if ((error as NodeJS.ErrnoException).code !== "ENOENT" || this.hasLoadedFile)
          throw error;
        missing = true;
      }
      if (missing) {
        this.current = { webhooks: {} };
        this.error = undefined;
        this.revision++;
        return this.current;
      }
      const jiti = createJiti(this.path, {
        moduleCache: false,
        tryNative: false,
        interopDefault: false,
        // Reuse the host runtime: live events must reach its AsyncLocalStorage.
        virtualModules: { runling, "runling/agents": agents, "runling/web": web },
      });
      const module = await jiti.import<{ default?: unknown }>(this.path);
      if (!isWebConfig(module.default))
        throw new Error(
          `${this.path} must export a valid Runling configuration`,
        );
      this.current = module.default;
      this.hasLoadedFile = true;
      this.error = undefined;
      this.revision++;
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
      serverLog("error", "config.reload_failed", { config: this.path, message: `Config reload failed: ${this.error}` });
      if (!this.current) throw error;
    } finally {
      for (const listener of this.listeners) listener();
    }
    return this.current!;
  }
  subscribe(listener: () => void) {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
  close() {
    this.disposed = true;
    clearTimeout(this.timer);
    this.listeners.clear();
    return this.watcher.close();
  }
}
