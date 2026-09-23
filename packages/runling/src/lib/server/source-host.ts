import { SourceManager } from "../../runtime/source-manager.ts";
import { getConfigReloader } from "./web-config.ts";
import { getRunStore } from "./run-store.ts";

const state = globalThis as typeof globalThis & {
  __runlingSourceHost?: Promise<{ close(): Promise<void> }>;
};

/** Eager singleton shared by the development server, packaged server, and HTTP routes. */
export function startSourceHost() {
  return state.__runlingSourceHost ??= (async () => {
    const config = getConfigReloader();
    const store = await getRunStore().catch(async error => { await config.close(); throw error; });
    const manager = new SourceManager(name => async (task, { input }) => {
      const { id } = await store.start(name, task, input, "source");
      return { id };
    });
    let revision = -1;
    const reload = async () => {
      const next = await config.load();
      if (config.revision === revision) return;
      revision = config.revision;
      await manager.replace(next);
    };
    try { await reload(); }
    catch (error) {
      await config.close();
      await manager.close();
      throw error;
    }
    const unsubscribe = config.subscribe(() => { void reload().catch(() => {}); });
    return { async close() {
      unsubscribe();
      await config.close();
      await manager.close();
      await store.close();
      delete state.__runlingSourceHost;
      delete (globalThis as typeof globalThis & { __runlingConfigReloader?: unknown }).__runlingConfigReloader;
    } };
  })().catch(error => {
    delete state.__runlingSourceHost;
    delete (globalThis as typeof globalThis & { __runlingConfigReloader?: unknown }).__runlingConfigReloader;
    throw error;
  });
}

/** Called by server shutdown, after new requests have been stopped. */
export async function stopSourceHost() {
  await (await state.__runlingSourceHost?.catch(() => undefined))?.close();
}
