import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  APP_ORIGIN,
  createFrontendProtocolHandler,
} from "./frontend_protocol.mjs";

async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), "chatto-desktop-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await writeFile(path.join(root, "200.html"), "shell");
  await writeFile(path.join(root, "asset.js"), "asset");
  return root;
}

test("serves assets and falls back to the SPA shell", async (t) => {
  const root = await fixture(t);
  const fetched = [];
  const handler = createFrontendProtocolHandler(root, async (input) => {
    fetched.push(String(input));
    return new Response("ok");
  });

  await handler(new Request(`${APP_ORIGIN}/asset.js`));
  await handler(
    new Request(`${APP_ORIGIN}/servers/callback?mode=popup`, {
      headers: { accept: "text/html" },
    }),
  );

  assert.match(fetched[0], /asset\.js$/);
  assert.match(fetched[1], /200\.html$/);
});

test("rejects other hosts on the custom scheme", async () => {
  const handler = createFrontendProtocolHandler("/unused", async () =>
    new Response("unexpected"),
  );

  const response = await handler(new Request("chatto://untrusted/asset.js"));
  assert.equal(response.status, 404);
});

test("does not serve files outside the frontend build", async (t) => {
  const root = await fixture(t);
  const handler = createFrontendProtocolHandler(
    root,
    async (input) => new Response(String(input)),
  );

  const response = await handler(
    new Request(`${APP_ORIGIN}/..%2Fpackage.json`),
  );
  assert.equal(response.status, 404);
});
