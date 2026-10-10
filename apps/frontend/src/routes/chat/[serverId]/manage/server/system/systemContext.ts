import { createContext } from 'svelte';
import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';

/**
 * The diagnostics snapshot that the System layout loads, shared with its
 * section pages.
 *
 * The layout owns the snapshot query and the refresh action. It renders the
 * section pages only while a snapshot is loaded, so `info` is always set.
 */
export interface SystemContext {
  readonly info: AdminSystemInfo;
}

const [getSystem, setSystem] = createContext<SystemContext>();

/** Provides the System context to the section pages. */
export function provideSystem(context: SystemContext): void {
  setSystem(context);
}

/** Returns the System context provided by the System layout. */
export function useSystem(): SystemContext {
  return getSystem();
}
