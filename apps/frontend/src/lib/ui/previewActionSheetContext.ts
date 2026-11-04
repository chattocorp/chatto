import { createContext } from 'svelte';

/** Sheet actions close their owning surface before invoking the preview callback. */
export const [usePreviewActionSheet, providePreviewActionSheet, hasPreviewActionSheet] =
  createContext<() => void>();
