import { describe, expect, test, vi } from "vitest";
import { createWebFetchExtension, fetchWithPinnedAddresses } from "./web-fetch.ts";
import { createServer } from "node:http";

function loadWebFetchTool(
  fetch: (input: URL, init: RequestInit, addresses: readonly string[]) => Promise<Response>,
  resolveAddresses: (hostname: string) => Promise<readonly string[]> = async () =>
    ["93.184.216.34"],
) {
  let registeredTool: any;
  createWebFetchExtension({ fetch, resolveAddresses })({
    registerTool(tool: unknown) {
      registeredTool = tool;
    },
  } as any);
  return registeredTool;
}

describe("web_fetch extension", () => {
  test("fetches textual HTTP content with response metadata", async () => {
    const fetch = vi.fn(async (_input: URL, _init: RequestInit, _addresses: readonly string[]) =>
      new Response("Hello from the web", {
        status: 200,
        statusText: "OK",
        headers: { "content-type": "text/plain; charset=utf-8" },
      }));
    const webFetchTool = loadWebFetchTool(fetch);
    const result = await webFetchTool.execute(
      "tool-call",
      { url: "https://example.com/page" },
      undefined,
    );

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[2]).toEqual(["93.184.216.34"]);
    expect(result.content[0].text).toContain("Status: 200 OK");
    expect(result.content[0].text).toContain("Hello from the web");
    expect(result.details).toMatchObject({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      truncated: false,
    });
  });

  test("rejects non-HTTP URLs before fetching", async () => {
    const webFetchTool = loadWebFetchTool(async () => {
      throw new Error("fetch should not run");
    });
    await expect(
      webFetchTool.execute(
        "tool-call",
        { url: "file:///etc/passwd" },
        undefined,
      ),
    ).rejects.toThrow("only supports HTTP and HTTPS");
  });

  test("truncates large responses", async () => {
    const webFetchTool = loadWebFetchTool(async () =>
      new Response("x".repeat(100_001), {
        headers: { "content-type": "text/plain" },
      }));
    const result = await webFetchTool.execute(
      "tool-call",
      { url: "https://example.com/large" },
      undefined,
    );

    expect(result.details.truncated).toBe(true);
    expect(result.content[0].text).toContain(
      "[Response truncated after 100000 bytes]",
    );
  });

  test.each([
    "127.0.0.1",
    "10.0.0.1",
    "169.254.169.254",
    "::1",
    "fd00::1",
  ])("blocks private destination %s", async (address) => {
    const fetch = vi.fn(async () => new Response("should not be fetched"));
    const webFetchTool = loadWebFetchTool(fetch, async () => [address]);

    await expect(
      webFetchTool.execute(
        "tool-call",
        { url: "https://internal.example" },
        undefined,
      ),
    ).rejects.toThrow("blocked non-public destination");
    expect(fetch).not.toHaveBeenCalled();
  });

  test("blocks a redirect to a private destination before following it", async () => {
    const fetch = vi.fn(async () =>
      new Response(null, {
        status: 302,
        headers: { location: "http://169.254.169.254/latest/meta-data" },
      }));
    const webFetchTool = loadWebFetchTool(fetch, async (hostname) =>
      hostname === "public.example" ? ["93.184.216.34"] : [hostname],
    );

    await expect(
      webFetchTool.execute(
        "tool-call",
        { url: "https://public.example/redirect" },
        undefined,
      ),
    ).rejects.toThrow("blocked non-public destination");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  test("pins a fresh set of checked addresses on each redirect", async () => {
    const fetch = vi.fn(async (_url: URL, _init: RequestInit, _addresses: readonly string[]) =>
      fetch.mock.calls.length === 1
        ? new Response(null, { status: 302, headers: { location: "/next" } })
        : new Response("done", { headers: { "content-type": "text/plain" } }));
    const resolveAddresses = vi.fn()
      .mockResolvedValueOnce(["93.184.216.34"])
      .mockResolvedValueOnce(["93.184.216.35"]);
    const tool = loadWebFetchTool(fetch, resolveAddresses);
    await tool.execute("call", { url: "https://public.example/start" });
    expect(resolveAddresses).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls.map((call) => call[2])).toEqual([
      ["93.184.216.34"], ["93.184.216.35"],
    ]);
  });

  test("connects to the pinned IP without resolving the hostname again", async () => {
    const hosts: string[] = [];
    const server = createServer((request, response) => {
      hosts.push(request.headers.host!);
      response.setHeader("content-type", "text/plain");
      response.end("pinned connection");
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const port = (server.address() as { port: number }).port;
    try {
      // The transport receives already-checked addresses. Use loopback only
      // here to prove the real connector does not look up this .invalid name.
      const response = await fetchWithPinnedAddresses(
        new URL(`http://does-not-resolve.invalid:${port}/`),
        { signal: AbortSignal.timeout(3000) },
        ["127.0.0.1"],
      );
      expect(await response.text()).toBe("pinned connection");
      expect(hosts).toEqual([`does-not-resolve.invalid:${port}`]);
    } finally {
      server.closeAllConnections();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });
});
