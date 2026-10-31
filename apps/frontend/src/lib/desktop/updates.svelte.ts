/** App-lifetime view of the optional host updater. Electron owns all persisted state. */
import { z } from 'zod';

const statusSchema = z.object({
  revision: z.number().int().nonnegative(),
  version: z.string().min(1).max(128),
  status: z.enum([
    'unavailable',
    'idle',
    'checking',
    'downloading',
    'ready',
    'current',
    'error',
    'changed',
    'installing'
  ]),
  busy: z.boolean(),
  progress: z.number().min(0).max(100).nullable(),
  availableVersion: z.string().min(1).max(128).nullable(),
  automatic: z.boolean(),
  channel: z.enum(['stable', 'beta'])
});

export type DesktopUpdateStatus = z.infer<typeof statusSchema>;

/** The preload exposes commands, never a feed URL, installer, or general IPC API. */
export interface DesktopUpdatesBridge {
  getStatus(): Promise<unknown>;
  onStatus(callback: (status: unknown) => void): () => void;
  check(): Promise<unknown>;
  setPreference(key: 'automatic' | 'channel', value: boolean | 'stable' | 'beta'): Promise<unknown>;
  install(): Promise<unknown>;
}

/** One owner for snapshots, pending commands, and the document's IPC subscription. */
export class DesktopUpdates {
  status = $state.raw<DesktopUpdateStatus | null>(null);
  loading = $state(true);
  pending = $state(false);
  failed = $state(false);
  private bridge: DesktopUpdatesBridge | undefined;
  private generation = 0;

  /** Host update information exists even when this installation cannot update itself. */
  get available() {
    return this.status !== null;
  }
  get busy() {
    return this.pending || this.status?.busy === true;
  }

  /** The root layout starts this subscription; disposal rejects late snapshots and commands. */
  start(bridge = typeof window === 'undefined' ? undefined : window.chattoDesktop?.updates) {
    const generation = ++this.generation;
    this.bridge = bridge;
    this.status = null;
    this.failed = false;
    this.loading = Boolean(bridge);
    const accept = (value: unknown) => {
      if (generation !== this.generation) return;
      const parsed = statusSchema.safeParse(value);
      if (!parsed.success) {
        if (value !== null) this.failed = true;
        return;
      }
      // Subscribe before fetching: a delayed read must not overwrite a newer event.
      if (this.status && parsed.data.revision < this.status.revision) return;
      this.status = parsed.data;
      this.failed = false;
    };
    const stop = bridge?.onStatus(accept);
    if (bridge)
      void bridge
        .getStatus()
        .then(accept)
        .catch(() => {
          if (generation === this.generation) this.failed = true;
        })
        .finally(() => {
          if (generation === this.generation) this.loading = false;
        });
    return () => {
      stop?.();
      if (generation !== this.generation) return;
      this.generation++;
      this.bridge = undefined;
      this.status = null;
      this.pending = false;
    };
  }

  private async run(action: (bridge: DesktopUpdatesBridge) => Promise<unknown>) {
    if (!this.bridge || !this.available || this.status?.status === 'unavailable' || this.busy)
      return;
    const generation = this.generation;
    this.pending = true;
    this.failed = false;
    try {
      const result = await action(this.bridge);
      if (result === false && generation === this.generation) this.failed = true;
    } catch {
      if (generation === this.generation) this.failed = true;
    } finally {
      if (generation === this.generation) this.pending = false;
    }
  }

  check() {
    return this.run((bridge) => bridge.check());
  }
  setAutomatic(value: boolean) {
    return this.run((bridge) => bridge.setPreference('automatic', value));
  }
  setChannel(value: string) {
    if (value !== 'stable' && value !== 'beta') return;
    return this.run((bridge) => bridge.setPreference('channel', value));
  }
  install() {
    return this.run((bridge) => bridge.install());
  }
}

export const desktopUpdates = new DesktopUpdates();
