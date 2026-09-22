import type { Handle, HandleServerError, ServerInit } from "@sveltejs/kit";
import { building } from "$app/environment";
import { startSourceHost, stopSourceHost } from "./lib/server/source-host.ts";
import { serverLog } from "./runtime/server-log.ts";

/** Adapter-node calls this during handler import, before the first request. */
export const init: ServerInit = async () => {
  if (building) return;
  await startSourceHost();
};

// The CLI awaits this hook before it closes its HTTP server.
if (!building) {
  (globalThis as typeof globalThis & { __runlingStopSources?: () => Promise<void> }).__runlingStopSources = stopSourceHost;
}

export const handle: Handle = async ({ event, resolve }) => {
  const started = performance.now();
  const response = await resolve(event);
  serverLog(response.status >= 500 ? "error" : response.status >= 400 ? "warn" : "info", "http.response", {
    method: event.request.method,
    route: event.route.id,
    status: response.status,
    durationMs: Math.round(performance.now() - started),
  });
  return response;
};

export const handleError: HandleServerError = ({ error, event, status }) => {
  serverLog("error", "http.error", {
    method: event.request.method,
    route: event.route.id,
    status,
    error,
  });
};
