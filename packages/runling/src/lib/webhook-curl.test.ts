import { spawnProcess } from "../../test/process.ts";
import { expect, test } from "vitest";
import { shellQuote, webhookCurl } from "./webhook-curl.ts";

test("shell quoting protects apostrophes and shell expressions", () => {
  expect(shellQuote("it's $(whoami)")).toBe("'it'\\''s $(whoami)'");
});

test("curl example preserves the URL and sample JSON as literal shell arguments", async () => {
  const url = "http://localhost:5173/api/webhooks/a%20hook?x='&y=2";
  const body = { value: "it's $(whoami) `pwd` $HOME\nnext line" };
  const command = webhookCurl(url, { examples: [body] });
  // Substitute a local function for curl. No HTTP request is sent.
  const process = spawnProcess(
    ["/bin/sh", "-c", `curl() { printf '%s\\0' "$@"; }; ${command}`],
    { stdout: "pipe", stderr: "pipe" },
  );
  const output = await new Response(process.stdout).text();
  expect(await process.exited).toBe(0);
  expect(output.split("\0")).toEqual([
    "--request",
    "POST",
    url,
    "--header",
    "content-type: application/json",
    "--data-raw",
    JSON.stringify(body, null, 2),
    "",
  ]);
});

test("curl example uses the workflow input schema", () => {
  expect(
    webhookCurl("http://localhost/hook", {
      type: "object",
      properties: { value: { type: "string", default: "monorepos" } },
    }),
  ).toContain('"value": "monorepos"');
});

test("string workflow inputs produce a JSON string payload, not an object", () => {
  expect(
    webhookCurl("http://localhost/hook", {
      type: "string",
      default: "monorepos",
    }),
  ).toContain(`--data-raw '"monorepos"'`);
});
