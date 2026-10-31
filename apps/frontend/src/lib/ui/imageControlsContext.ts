import { createContext } from 'svelte';

/** The viewer owns the footer target; its image stage owns the controls. */
export const [useImageControls, provideImageControls, hasImageControls] =
  createContext<() => HTMLElement | undefined>();
