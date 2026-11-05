/** Native host lifecycle. Chatto's SDK owns auth renewal, requests, paging, and realtime state. */
import '@chatto/client/svelte';
import { createClient, passwordLogin, type ChattoClient, type Server } from '@chatto/client';
import { emptyServerSession } from '@chatto/client/server/sessions';
import { persistedBearerSession } from '@chatto/client/auth/bearerSession';
import { signOutServer } from '@chatto/client/auth/signOut';
import { promptPassword } from './native-auth.ts';
import { readPreferences, savePreferences } from './preferences.ts';

export class NativeSession {
  server = $state.raw<Server | null>(null);
  selected = $state('');
  origin = $state('');
  login = $state('');
  busy = $state(false);
  error = $state('');
  draftGeneration = $state(0);
  #client: ChattoClient | null = null;
  #operation: AbortController | null = null;
  #epoch = 0;
  #dispose: (() => void)[] = [];
  #password: (signal?: AbortSignal) => Promise<string | null>;

  constructor(options: { password?: (signal?: AbortSignal) => Promise<string | null> } = {}) {
    const saved = readPreferences();
    this.origin = saved.origin;
    this.login = saved.login;
    this.selected = saved.room;
    this.#password = options.password ?? promptPassword;
  }
  #save() {
    savePreferences({ origin: this.origin, login: this.login, room: this.selected });
  }
  /** Save only the native navigation choice; the SDK checks room access. */
  select(room: string) {
    this.selected = room;
    this.error = '';
    try {
      this.#save();
    } catch {
      this.error = 'Could not save connection settings';
    }
  }
  /** Use secure host input, then register a renewable session with the SDK. */
  async signIn(): Promise<void> {
    if (this.busy) return;
    this.busy = true;
    this.error = '';
    const epoch = ++this.#epoch;
    const controller = new AbortController();
    this.#operation = controller;
    let client: ChattoClient | null = null;
    try {
      const password = await this.#password(controller.signal);
      if (epoch !== this.#epoch || password === null) return;
      const credentials = await passwordLogin(
        this.origin,
        { login: this.login, password },
        { signal: controller.signal }
      );
      if (epoch !== this.#epoch) return;
      client = createClient({
        storage: 'memory',
        liveServers: 'selected',
        keepRecent: { rooms: 1, threads: 1 }
      });
      const id = `gpuix-${crypto.randomUUID()}`;
      const origin = new URL(this.origin).origin;
      client.registry.addServer(
        { id, url: origin, name: 'Chatto', iconUrl: null, addedAt: Date.now() },
        { ...emptyServerSession(), ...persistedBearerSession(credentials) }
      );
      const server = client.server(id);
      if (!server) throw new Error('The connection could not be created');
      this.#client = client;
      this.server = server;
      client = null;
      const clear = () => {
        this.draftGeneration++;
      };
      this.#dispose = [
        server.onReset(clear),
        server.onRoomAccessLost(clear),
        server.onAuthorityChanged(clear),
        server.onPermissionsChanged(clear),
        server.onUserDeleted(clear),
        server.onSessionEnded(() => {
          this.close();
          this.error = 'Your session ended. Sign in again.';
        })
      ];
      this.#client.start();
      this.#client.setActiveServer(id);
      this.origin = origin;
      try {
        this.#save();
      } catch {
        this.error = 'Connected, but settings could not be saved';
      }
    } catch (error) {
      client?.close();
      if (epoch === this.#epoch)
        this.error = error instanceof Error ? error.message : 'Could not sign in';
    } finally {
      if (epoch === this.#epoch) {
        this.busy = false;
        this.#operation = null;
      }
    }
  }
  /** Clear the native view first, then ask the SDK to revoke the session. */
  async signOut(): Promise<void> {
    if (this.busy) return;
    const server = this.server;
    const registration = server ? this.#client?.registry.getServer(server.serverId) : null;
    this.close();
    this.busy = true;
    try {
      if (registration) await signOutServer(registration, false);
    } catch {
      this.error = 'Signed out locally. Server revocation could not be completed.';
    } finally {
      this.busy = false;
    }
  }
  /** Cancel delivery and discard private server data before closing its SDK runtime. */
  close(): void {
    ++this.#epoch;
    this.#operation?.abort();
    this.#operation = null;
    for (const dispose of this.#dispose) dispose();
    this.#dispose = [];
    this.server = null;
    this.draftGeneration++;
    this.#client?.close();
    this.#client = null;
    this.busy = false;
  }
}
