import assert from "node:assert/strict";
import { createFrontendHandler } from "./frontend_server.ts";
import { oauthNavigationUrl } from "./oauth_window.ts";

Deno.test("serves the official frontend assets and SPA fallback", async () => {
  const root = await Deno.makeTempDir();
  try {
    await Deno.mkdir(`${root}/_app`, { recursive: true });
    await Deno.writeTextFile(`${root}/200.html`, "official-fallback");
    await Deno.writeTextFile(`${root}/_app/client.js`, "official-client");

    const handler = createFrontendHandler(root);
    const rootResponse = await handler(
      new Request("http://127.0.0.1:4567/"),
    );
    assert.equal(rootResponse.status, 200);
    assert.equal(await rootResponse.text(), "official-fallback");

    const asset = await handler(
      new Request("http://127.0.0.1:4567/_app/client.js"),
    );
    assert.equal(asset.status, 200);
    assert.equal(await asset.text(), "official-client");

    const route = await handler(
      new Request("http://127.0.0.1:4567/chat/example.test", {
        headers: { accept: "text/html" },
      }),
    );
    assert.equal(route.status, 200);
    assert.equal(await route.text(), "official-fallback");

    const missingAsset = await handler(
      new Request("http://127.0.0.1:4567/_app/missing.js"),
    );
    assert.equal(missingAsset.status, 404);
  } finally {
    await Deno.remove(root, { recursive: true });
  }
});

Deno.test("allows only credential-free HTTP OAuth navigation", () => {
  assert.equal(
    oauthNavigationUrl("https://chat.example/oauth/authorize?state=abc"),
    "https://chat.example/oauth/authorize?state=abc",
  );
  assert.equal(
    oauthNavigationUrl("http://127.0.0.1:8080/oauth/authorize"),
    "http://127.0.0.1:8080/oauth/authorize",
  );
  assert.throws(() => oauthNavigationUrl("file:///tmp/secret"), TypeError);
  assert.throws(
    () => oauthNavigationUrl("https://user:password@chat.example/oauth"),
    TypeError,
  );
  assert.throws(() => oauthNavigationUrl(42), TypeError);
});
