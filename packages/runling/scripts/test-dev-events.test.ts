import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { test } from "vitest";

test("Vite dev workflows record task and input events in the web host", async () => {
  await promisify(execFile)(process.execPath, [
    "--import", "tsx", "--conditions=runling-source", "scripts/test-dev-events.mjs",
  ], { timeout: 20_000 });
}, 25_000);
