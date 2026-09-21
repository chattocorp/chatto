#!/usr/bin/env node
import "tsx/esm";
import { existsSync, readFileSync } from "node:fs";
import { registerHooks } from "node:module";
import { CommanderError } from "commander";
import { createCli } from "#cli";
import { resolve } from "./workflow-loader.js";

registerHooks({ resolve });
const { version } = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
const loadEnv = () => {
  if (existsSync(".env")) process.loadEnvFile(".env");
};

try {
  await createCli(version, {
    async run(file, prompt, options) {
      loadEnv();
      const { runRunling } = await import("#runner");
      await runRunling(file, prompt, options);
    },
    async serve(options) {
      loadEnv();
      const { runRunlingWeb } = await import("../dist/src/runtime/web-server.js");
      await runRunlingWeb(options);
    },
  }).parseAsync(process.argv);
} catch (error) {
  if (error instanceof CommanderError) {
    process.exitCode = error.exitCode;
  } else {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
