import type { ConfigReloader } from 'runling/config-reloader';
// Keep the loader's virtual modules in the installed runtime, outside the console bundle.
const loaderModule = 'runling/config-reloader';
const { ConfigReloader: Loader } = (await import(
  /* @vite-ignore */ loaderModule
)) as typeof import('runling/config-reloader');
// The source host and HTTP routes must share configuration across Vite reloads.
const state = globalThis as typeof globalThis & { __runlingConfigReloader?: ConfigReloader };
export function getConfigReloader(): ConfigReloader {
  const path = process.env.RUNLING_WEB_CONFIG;
  if (!path) throw new Error('RUNLING_WEB_CONFIG is not set');
  return (state.__runlingConfigReloader ??= new Loader(path, {
    watch: process.env.RUNLING_WATCH === '1'
  }));
}
export const loadWebConfig = () => getConfigReloader().load();
