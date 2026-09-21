import type { Handle, HandleServerError } from "@sveltejs/kit";
import { serverLog } from "./runtime/server-log.ts";

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
