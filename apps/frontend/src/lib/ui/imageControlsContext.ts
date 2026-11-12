import { createContext } from 'svelte';

/** Footer target for animated diagram playback controls. */
export const [useImageControls, provideImageControls, hasImageControls] =
  createContext<() => HTMLElement | undefined>();

/** Live zoom state and commands owned by one mounted image stage. */
export interface ImageZoomController {
  readonly percent: number;
  readonly atMinimum: boolean;
  readonly atMaximum: boolean;
  /** Exact Fit state, independent of the rounded percentage label. */
  readonly atFit: boolean;
  zoomIn(): void;
  zoomOut(): void;
  fit(): void;
}

/** Register a stage with the viewer toolbar and unregister it on disposal. */
export const [useImageZoomControls, provideImageZoomControls, hasImageZoomControls] =
  createContext<(controls: ImageZoomController) => () => void>();
