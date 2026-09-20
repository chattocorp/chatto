import { serverLog, serverLogPath } from "./server-log.ts";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import type { ServeOptions } from "./cli.ts";
import { createServer, type RequestListener } from "node:http";

export async function runRunlingWeb(options: ServeOptions) {
  const configPath = resolve(options.config);
  process.env.RUNLING_WEB_CONFIG = configPath;
  serverLog("info", "server.starting", { config: configPath, logFile: serverLogPath(), port: options.port });
  try {
    process.env.HOST = options.host;
    process.env.PORT = String(options.port);
    // Import only after setting the adapter's startup environment. Keep the project cwd.
    const serverUrl = new URL("../../web/handler.js", import.meta.url);
    if (!existsSync(serverUrl))
      throw new Error(
        "Runling web assets are missing. Build the package before running it.",
      );
    const { handler } = (await import(/* @vite-ignore */ serverUrl.href)) as {
      handler: RequestListener;
    };
    const server = createServer(handler);
    await new Promise<void>((resolve, reject) => {
      server.once("error", reject);
      server.listen(options.port, options.host, () => {
        server.off("error", reject);
        resolve();
      });
    });
    const host = options.host.includes(":") ? `[${options.host}]` : options.host;
    serverLog("info", "server.listening", { url: `http://${host}:${options.port}` });
    const shutdown = () => {
      serverLog("info", "server.stopping");
      server.close(() => {
        serverLog("info", "server.stopped");
        process.off("SIGINT", shutdown);
        process.off("SIGTERM", shutdown);
      });
      setTimeout(() => server.closeAllConnections(), 5000).unref();
    };
    process.once("SIGINT", shutdown);
    process.once("SIGTERM", shutdown);
    if (options.open) {
      const { spawn } = await import("node:child_process");
      const host = options.host.includes(":")
        ? `[${options.host}]`
        : options.host;
      const url = `http://${host}:${options.port}`;
      const child =
        process.platform === "darwin"
          ? spawn("open", [url])
          : process.platform === "win32"
            ? spawn("explorer.exe", [url])
            : spawn("xdg-open", [url]);
      child.on("error", () => serverLog("warn", "server.browser_failed", { url }));
      child.unref();
    }
  } catch (error) {
    serverLog("error", "server.start_failed", { error });
    throw error;
  }
}
