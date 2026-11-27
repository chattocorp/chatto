import { createContext } from 'svelte';

/** Controls the role picker for one mounted server session. */
export const [useShieldMode, provideShieldMode] = createContext<{
  readonly loading: boolean;
  open(afterActivation?: () => void): void;
  deactivate(): Promise<void>;
}>();
