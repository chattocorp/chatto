import { expect, test, vi } from "vitest";
import { fetchDocsPage } from "./docs.ts";

const html = (body: string) => new Response(`<html><head><title>Chatto guide</title></head><body>${body}</body></html>`,
  { headers: { "content-type": "text/html; charset=utf-8" } });

test("returns readable content, canonical request URL, and same-site links without executing assets", async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(html('<nav><a href="/guide/">Guide</a></nav><main><h1>Hello &amp; welcome</h1><p>Read this.</p><script>secret script</script><a href="https://other.example">External</a></main>'));
  const page = await fetchDocsPage("https://docs.chatto.run/#top", undefined, request);
  expect(page).toMatchObject({ title: "Chatto guide", url: "https://docs.chatto.run/", truncated: false,
    links: [{ title: "Guide", url: "https://docs.chatto.run/guide/" }] });
  expect(page.text).toContain("Hello & welcome\n");
  expect(page.text).not.toContain("secret script");
  expect(request).toHaveBeenCalledOnce();
  expect(request.mock.calls[0]![1]).toMatchObject({ redirect: "manual", credentials: "omit" });
});

test.each(["https://evil.example/", "http://docs.chatto.run/", "https://docs.chatto.run.evil.example/",
  "https://secret@docs.chatto.run/", "https://docs.chatto.run/?secret=value", "https://docs.chatto.run:444/", "file:///etc/passwd"])("rejects %s before fetching", async url => {
  const request = vi.fn<typeof fetch>();
  await expect(fetchDocsPage(url, undefined, request)).rejects.toThrow();
  expect(request).not.toHaveBeenCalled();
});

test("checks redirect targets and follows allowed relative redirects", async () => {
  const request = vi.fn<typeof fetch>()
    .mockResolvedValueOnce(new Response(null, { status: 302, headers: { location: "/guide/" } }))
    .mockResolvedValueOnce(html("<main>Guide</main>"));
  expect((await fetchDocsPage("/", undefined, request)).url).toBe("https://docs.chatto.run/guide/");
  request.mockReset().mockResolvedValue(new Response(null, { status: 302, headers: { location: "https://evil.example/" } }));
  await expect(fetchDocsPage("/", undefined, request)).rejects.toThrow("Only HTTPS");
  expect(request).toHaveBeenCalledOnce();
});

test("bounds redirect loops, body bytes, and returned text", async () => {
  const request = vi.fn<typeof fetch>().mockImplementation(async () => new Response(null, { status: 302, headers: { location: "/" } }));
  await expect(fetchDocsPage("/", undefined, request)).rejects.toThrow("redirect limit");
  expect(request).toHaveBeenCalledTimes(5);
  request.mockResolvedValue(html("x".repeat(512_001)));
  await expect(fetchDocsPage("/", undefined, request)).rejects.toThrow("size limit");
  request.mockResolvedValue(html(`<main>${"x".repeat(31_000)}</main>`));
  const page = await fetchDocsPage("/", undefined, request);
  expect(page.text).toHaveLength(30_000);
  expect(page.truncated).toBe(true);
});

test("rejects failed or non-HTML responses", async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(new Response("no", { status: 404 }));
  await expect(fetchDocsPage("/", undefined, request)).rejects.toThrow("unavailable");
  request.mockResolvedValue(new Response("binary", { headers: { "content-type": "image/png" } }));
  await expect(fetchDocsPage("/", undefined, request)).rejects.toThrow("not HTML");
});

test.each(["cancel", "timeout"])("stops an in-flight request on %s", async mode => {
  vi.useFakeTimers();
  vi.spyOn(AbortSignal, "timeout").mockImplementation(ms => {
    expect(ms).toBe(15_000);
    const controller = new AbortController();
    setTimeout(() => controller.abort(), ms);
    return controller.signal;
  });
  try {
    const controller = new AbortController();
    const request = vi.fn<typeof fetch>().mockImplementation(async (_url, init) => {
      expect(init!.signal).not.toBe(controller.signal);
      return new Promise((_resolve, reject) => init!.signal!.addEventListener("abort", () => reject(new Error("aborted"))));
    });
    const result = fetchDocsPage("/", controller.signal, request);
    const rejected = expect(result).rejects.toThrow("cancelled");
    if (mode === "cancel") controller.abort();
    else await vi.advanceTimersByTimeAsync(15_000);
    await rejected;
  } finally { vi.restoreAllMocks(); vi.useRealTimers(); }
});
