#!/usr/bin/env node
import 'tsx/esm';
import { existsSync, readFileSync } from 'node:fs';
import { registerHooks } from 'node:module';
import { CommanderError } from 'commander';
import { resolve } from './workflow-loader.js';

registerHooks({ resolve });
// Workspace links use source; the published tarball contains only compiled runtime files.
const runtime = existsSync(new URL('../src/runtime/cli.ts', import.meta.url))
  ? '../src/runtime/'
  : '../dist/src/runtime/';
const extension = runtime.startsWith('../src/') ? 'ts' : 'js';
const loadRuntime = (name) =>
  import(new URL(`${runtime}${name}.${extension}`, import.meta.url).href);
const { createCli } = await loadRuntime('cli');
const { version } = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8'));
const loadEnv = () => {
  if (existsSync('.env')) process.loadEnvFile('.env');
};

try {
  await createCli(version, {
    async run(file, prompt, options) {
      loadEnv();
      const { runRunling } = await loadRuntime('runner');
      await runRunling(file, prompt, options);
    },
    async serve(options) {
      loadEnv();
      const { runRunlingWeb } = await loadRuntime('web-server');
      await runRunlingWeb(options);
    }
  }).parseAsync(process.argv);
} catch (error) {
  if (error instanceof CommanderError) {
    process.exitCode = error.exitCode;
  } else {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
