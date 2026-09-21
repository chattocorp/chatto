import { expect, it, vi } from "vitest";
import type { RequestEvent } from "@sveltejs/kit";
import { handle, handleError } from "./hooks.server.ts";
import { serverLog } from "./runtime/server-log.ts";

vi.mock("./runtime/server-log.ts", () => ({ serverLog: vi.fn() }));

it("logs response metadata without reading bodies or exposing URLs and headers", async () => {
  vi.mocked(serverLog).mockClear();
  const event = {
    request: new Request("http://localhost/api/webhooks/private?key=secret", {
      method: "POST", headers: { authorization: "Bearer secret" }, body: "private message",
    }),
    route: { id: "/api/webhooks/[name]" },
  } as RequestEvent;
  const stream = new ReadableStream();
  const response = new Response(stream, { headers: { "content-type": "text/event-stream" } });
  expect(await handle({ event, resolve: async () => response })).toBe(response);
  expect(event.request.bodyUsed).toBe(false);
  expect(serverLog).toHaveBeenCalledExactlyOnceWith("info", "http.response", {
    method: "POST", route: "/api/webhooks/[name]", status: 200, durationMs: expect.any(Number),
  });
  await stream.cancel();
});

it("logs unexpected request errors", () => {
  vi.mocked(serverLog).mockClear();
  const error = new Error("failed to load config");
  handleError({ error, event: { request: { method: "GET" }, route: { id: "/" } } as RequestEvent, status: 500, message: "Internal Error" });
  expect(serverLog).toHaveBeenCalledWith("error", "http.error", { method: "GET", route: "/", status: 500, error });
});
