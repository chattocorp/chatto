import type {
  ExtensionAPI,
  ToolDefinition,
} from "@earendil-works/pi-coding-agent";
import assert from "node:assert/strict";
import test from "node:test";
import { createWebFetchExtension, pinnedPublicLookup } from "./web-fetch.js";

function loadWebFetchTool(
  fetch: (
    input: URL,
    init: RequestInit,
    addresses: readonly string[],
  ) => Promise<Response>,
  resolveAddresses: (
    hostname: string,
  ) => Promise<readonly string[]> = async () => ["93.184.216.34"],
): ToolDefinition {
  let registeredTool: ToolDefinition | undefined;
  createWebFetchExtension({ fetch, resolveAddresses })({
    registerTool(tool: ToolDefinition) {
      registeredTool = tool;
    },
  } as unknown as ExtensionAPI);
  assert.ok(registeredTool);
  return registeredTool;
}

async function executeWebFetch(
  tool: ToolDefinition,
  url: string,
): Promise<Awaited<ReturnType<ToolDefinition["execute"]>>> {
  return tool.execute(
    "tool-call",
    { url },
    undefined,
    undefined,
    undefined as never,
  );
}

test("fetches textual public content with response metadata", async () => {
  let calls = 0;
  const tool = loadWebFetchTool(async () => {
    calls += 1;
    return new Response("Hello from the web", {
      status: 200,
      statusText: "OK",
      headers: { "content-type": "text/plain; charset=utf-8" },
    });
  });

  const result = await executeWebFetch(tool, "https://example.com/page");

  assert.equal(calls, 1);
  assert.match(
    result.content[0]?.type === "text" ? result.content[0].text : "",
    /Status: 200 OK/,
  );
  assert.match(
    result.content[0]?.type === "text" ? result.content[0].text : "",
    /Hello from the web/,
  );
  assert.deepEqual(result.details, {
    url: "",
    status: 200,
    contentType: "text/plain; charset=utf-8",
    truncated: false,
  });
});

test("pins each redirect to its validated addresses without a second DNS read", async () => {
  const resolved: string[] = [];
  const fetched: string[] = [];
  const tool = loadWebFetchTool(
    async (url, _init, addresses) => {
      const expected = fetched.length === 0 ? "93.184.216.34" : "1.1.1.1";
      assert.deepEqual(addresses, [expected]);
      const lookup = pinnedPublicLookup(addresses);
      // A hostname that resolves to loopback cannot alter the pinned answer.
      lookup("localhost", {}, (error, address, family) => {
        assert.equal(error, null);
        assert.equal(address, expected);
        assert.equal(family, 4);
      });
      lookup("localhost", { all: true }, (error, records) => {
        assert.equal(error, null);
        assert.deepEqual(records, [{ address: expected, family: 4 }]);
      });
      fetched.push(url.hostname);
      return fetched.length === 1
        ? new Response(null, {
            status: 302,
            headers: { location: "https://next.example/" },
          })
        : new Response("done", { headers: { "content-type": "text/plain" } });
    },
    async (hostname) => {
      resolved.push(hostname);
      // Any repeated DNS read returns private data and must not be used.
      if (resolved.filter((value) => value === hostname).length > 1)
        return ["127.0.0.1"];
      return [hostname === "first.example" ? "93.184.216.34" : "1.1.1.1"];
    },
  );
  await executeWebFetch(tool, "https://first.example/");
  assert.deepEqual(resolved, ["first.example", "next.example"]);
  assert.deepEqual(fetched, resolved);
});

test("pinned lookup rejects unsafe inputs and unsupported address families", () => {
  for (const addresses of [
    [],
    ["127.0.0.1"],
    ["240.0.0.1"],
    ["93.184.216.34", "::1"],
  ]) {
    assert.throws(
      () => pinnedPublicLookup(addresses),
      /public destination addresses/,
    );
  }
  const lookup = pinnedPublicLookup(["93.184.216.34"]);
  lookup("example.com", { family: 6 }, (error) => {
    assert.match(error?.message ?? "", /no approved address/);
  });
});

test("rejects non-HTTP URLs before resolving or fetching", async () => {
  let calls = 0;
  const tool = loadWebFetchTool(async () => {
    calls += 1;
    return new Response("should not be fetched");
  });

  await assert.rejects(
    executeWebFetch(tool, "file:///etc/passwd"),
    /only supports HTTP and HTTPS/,
  );
  assert.equal(calls, 0);
});

test("rejects credentials in a URL before resolving or fetching", async () => {
  let calls = 0;
  const tool = loadWebFetchTool(async () => {
    calls += 1;
    return new Response("should not be fetched");
  });

  await assert.rejects(
    executeWebFetch(tool, "https://user:secret@example.com/"),
    /does not accept credentials/,
  );
  assert.equal(calls, 0);
});

for (const address of [
  "127.0.0.1",
  "10.0.0.1",
  "169.254.169.254",
  "::1",
  "::ffff:127.0.0.1",
  "fd00::1",
  "2001:db8::1",
  "2002:7f00:1::",
]) {
  test(`blocks non-public destination ${address}`, async () => {
    let calls = 0;
    const tool = loadWebFetchTool(
      async () => {
        calls += 1;
        return new Response("should not be fetched");
      },
      async () => [address],
    );

    await assert.rejects(
      executeWebFetch(tool, "https://internal.example"),
      /blocked non-public destination/,
    );
    assert.equal(calls, 0);
  });
}

test("allows a public IPv6 destination", async () => {
  let calls = 0;
  const tool = loadWebFetchTool(
    async () => {
      calls += 1;
      return new Response("public IPv6", {
        headers: { "content-type": "text/plain" },
      });
    },
    async () => ["2606:4700:4700::1111"],
  );

  await executeWebFetch(tool, "https://example.com/");
  assert.equal(calls, 1);
});

test("blocks a redirect to a non-public destination", async () => {
  let calls = 0;
  const tool = loadWebFetchTool(
    async () => {
      calls += 1;
      return new Response(null, {
        status: 302,
        headers: { location: "http://169.254.169.254/latest/meta-data" },
      });
    },
    async (hostname) =>
      hostname === "public.example" ? ["93.184.216.34"] : [hostname],
  );

  await assert.rejects(
    executeWebFetch(tool, "https://public.example/redirect"),
    /blocked non-public destination/,
  );
  assert.equal(calls, 1);
});

test("rejects non-textual responses", async () => {
  const tool = loadWebFetchTool(
    async () =>
      new Response(new Uint8Array([0, 1, 2]), {
        headers: { "content-type": "application/octet-stream" },
      }),
  );

  await assert.rejects(
    executeWebFetch(tool, "https://example.com/file.bin"),
    /cannot return content type application\/octet-stream/,
  );
});

test("truncates large textual responses", async () => {
  const tool = loadWebFetchTool(
    async () =>
      new Response("x".repeat(100_001), {
        headers: { "content-type": "text/plain" },
      }),
  );
  const result = await executeWebFetch(tool, "https://example.com/large");
  const content = result.content[0];

  assert.equal((result.details as { truncated: boolean }).truncated, true);
  assert.match(
    content?.type === "text" ? content.text : "",
    /Response truncated after 100000 bytes/,
  );
});
