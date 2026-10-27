// Load host services outside the adapter bundle; application logs share their async context.
const hostModule = 'runling/server';
const { SourceManager } = (await import(
  /* @vite-ignore */ hostModule
)) as typeof import('runling/server');
import { getConfigReloader } from './web-config.ts';
import { getRunStore } from './run-store.ts';

const state = globalThis as typeof globalThis & {
  __runlingSourceHost?: Promise<{ close(): Promise<void> }>;
  __runlingRunStore?: ReturnType<typeof getRunStore>;
  __runlingConfigReloader?: ReturnType<typeof getConfigReloader>;
};

/** Eager singleton shared by the development server, packaged server, and HTTP routes. */
export function startSourceHost() {
  return (state.__runlingSourceHost ??= (async () => {
    const config = getConfigReloader();
    const store = await getRunStore().catch(async (error) => {
      try {
        await config.close();
      } finally {
        delete state.__runlingRunStore;
      }
      throw error;
    });
    const manager = new SourceManager(
      (name) =>
        (task, { input, runId }) =>
          store.start(name, task, input, 'source', runId),
      (name, options) => store.sourceRuns(name, options)
    );
    let unsubscribe = () => {};
    // Release every owner even when the final snapshot cannot be saved. A later
    // host must never reuse a rejected store or a closed source generation.
    const close = async () => {
      unsubscribe();
      try {
        try {
          await config.close();
        } finally {
          try {
            await manager.close();
          } finally {
            await store.close();
          }
        }
      } finally {
        delete state.__runlingRunStore;
        delete state.__runlingSourceHost;
        delete state.__runlingConfigReloader;
      }
    };
    let revision = -1;
    const reload = async () => {
      const next = await config.load();
      if (config.revision === revision) return;
      revision = config.revision;
      await store.restore(next.workflows);
      await manager.replace(next);
    };
    try {
      await reload();
    } catch (error) {
      await close();
      throw error;
    }
    unsubscribe = config.subscribe(() => {
      void reload().catch(() => {});
    });
    return { close };
  })().catch((error) => {
    delete state.__runlingSourceHost;
    delete state.__runlingConfigReloader;
    throw error;
  }));
}

/** Called by server shutdown, after new requests have been stopped. */
export async function stopSourceHost() {
  await (await state.__runlingSourceHost?.catch(() => undefined))?.close();
}
