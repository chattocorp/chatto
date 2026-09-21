import { spawnProcess } from "./process.ts";
import { describe, expect, test } from "vitest";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { readFile } from "node:fs/promises";

describe("web_fetch package integration", () => {
  test("the manifest loads only the selectable web_fetch extension", async () => {
    const packageJson = JSON.parse(await readFile(
      resolve(import.meta.dirname, "../package.json"),
      "utf8",
    ));
    expect(packageJson.pi.extensions).toEqual(["./dist/extensions/web-fetch.js"]);

    const temporaryDirectory = await mkdtemp(
      resolve(tmpdir(), "runling-web-fetch-"),
    );
    try {
      const child = spawnProcess(
        [
          process.execPath,
          resolve(import.meta.dirname, "fixtures/web-fetch-loader.ts"),
          resolve(import.meta.dirname, "../dist/extensions/web-fetch.js"),
          temporaryDirectory,
        ],
        { stdout: "pipe", stderr: "pipe" },
      );
      const [exitCode, stdout, stderr] = await Promise.all([
        child.exited,
        new Response(child.stdout).text(),
        new Response(child.stderr).text(),
      ]);

      expect(exitCode, stderr).toBe(0);
      expect(JSON.parse(stdout)).toEqual({
        errors: [],
        loadedTools: ["web_fetch"],
        selectedTools: ["web_fetch"],
      });
    } finally {
      await rm(temporaryDirectory, { recursive: true, force: true });
    }
  });
});
