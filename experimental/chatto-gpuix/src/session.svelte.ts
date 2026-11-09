/** Native multi-server host. The SDK owns sessions, renewal, requests and realtime state. */
import '@chatto/client/svelte';
import { createClient, passwordLogin, type ChattoClient, type Server } from '@chatto/client';
import type { NewBearerSession } from '@chatto/client/auth/bearerSession';
import { emptyServerSession } from '@chatto/client/server/sessions';
import { persistedBearerSession } from '@chatto/client/auth/bearerSession';
import { signOutServer } from '@chatto/client/auth/signOut';
import { getPublicServerInfo, type PublicServerInfo } from '@chatto/client/api/server';
import { nativeServerOrigin } from '@chatto/client/auth/nativeOrigin';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { appearance, changeScale, normalizeScale } from './ui/scale.svelte.ts';
import { applyTheme } from './ui/theme.ts';
import { promptPassword } from './native-auth.ts';
import { browserSignIn } from './browser-auth.ts';
import { readPreferences } from './preferences.ts';
import { openNativeState, type NativeState } from './state.ts';

/** Temporary built-in desktop identity for this native experiment. */
export const NATIVE_OAUTH_CLIENT_ID = 'chatto://desktop';
type Options = {
  password?: (signal?: AbortSignal) => Promise<string | null>;
  authorize?: typeof browserSignIn;
  openState?: () => Promise<NativeState>;
  clientId?: string;
};

export class NativeSession {
  activeServerId = $state('');
  origin = $state('');
  login = $state('');
  busy = $state(false);
  booting = $state(true);
  adding = $state(false);
  managing = $state(false);
  error = $state('');
  draftGeneration = $state(0);
  #client = $state.raw<ChattoClient | null>(null);
  #rooms = $state<Record<string, string>>({});
  #order = $state<string[]>([]);
  #state: NativeState | null = null;
  #ready: Promise<void>;
  #closed = false;
  #closing: Promise<void> | null = null;
  #operation: AbortController | null = null;
  #epoch = 0;
  #dispose: (() => void) | null = null;
  #options: Options;

  constructor(options: Options = {}) {
    const saved = readPreferences();
    this.origin = saved.origin;
    this.login = saved.login;
    this.#options = options;
    this.#ready = this.#initialize();
  }
  /** Finish encrypted storage restoration before signing in or rendering saved servers. */
  ready(): Promise<void> {
    return this.#ready;
  }
  get storageReady(): boolean {
    return this.#client !== null;
  }
  /** Retry after the user unlocks Keychain or closes the other app instance. */
  retryOpening(): void {
    if (!this.#client && !this.booting && !this.#closed) {
      this.booting = true;
      this.error = '';
      this.#ready = this.#initialize();
    }
  }
  get servers(): RegisteredServer[] {
    const servers = this.#client?.registry.servers ?? [];
    const position = (id: string) => {
      const index = this.#order.indexOf(id);
      return index < 0 ? Number.MAX_SAFE_INTEGER : index;
    };
    return servers.toSorted((a, b) => position(a.id) - position(b.id));
  }
  get selected(): string {
    return this.#rooms[this.activeServerId] ?? '';
  }
  get activeRegistration(): RegisteredServer | undefined {
    return this.#client?.registry.getServer(this.activeServerId);
  }
  get server(): Server | null {
    const record = this.activeRegistration;
    return !this.adding && !this.managing && record?.token && record.reauthRequiredAt === null
      ? (this.#client?.server(record.id) ?? null)
      : null;
  }
  /** Read a registered server for gutter notifications, without copying its stores. */
  serverFor(id: string): Server | undefined {
    return this.#client?.server(id);
  }

  async #initialize(): Promise<void> {
    let state: NativeState | null = null;
    let client: ChattoClient | null = null;
    try {
      state = await (this.#options.openState ?? openNativeState)();
      if (this.#closed) {
        await state.close();
        return;
      }
      client = createClient({
        persistence: state,
        liveServers: 'all',
        keepRecent: { rooms: 1, threads: 1 }
      });
      const ui = state.ui;
      changeScale(ui.uiScale ?? 1);
      applyTheme();
      this.#rooms = ui.rooms;
      this.#order = ui.order;
      this.activeServerId =
        client.registry.getServer(ui.activeServerId)?.id ?? client.registry.servers[0]?.id ?? '';
      this.#state = state;
      this.#client = client;
      this.#dispose = client.registry.watchStores((server) => {
        const clear = () => {
          if (this.activeServerId === server.serverId) this.draftGeneration++;
        };
        const detach = [
          server.onReset(clear),
          server.onRoomAccessLost(clear),
          server.onAuthorityChanged(clear),
          server.onPermissionsChanged(clear),
          server.onUserDeleted(clear),
          server.onSessionEnded(clear)
        ];
        return () => detach.forEach((dispose) => dispose());
      });
      // Restored registrations have no stores until init(). Install boundary
      // listeners first, then create those stores before the UI reads them.
      client.registry.init();
      client.start();
      client.setActiveServer(this.activeServerId || null);
      if (this.activeRegistration) this.origin = this.activeRegistration.url;
    } catch {
      client?.close();
      if (this.#state === state) {
        this.#client = null;
        this.#state = null;
      }
      await state?.close();
      this.error =
        'Could not open saved sessions securely. Check Keychain access and close other app instances.';
    } finally {
      this.booting = false;
    }
  }
  #save(): void {
    this.#state?.setUi({
      uiScale: appearance.scale,
      activeServerId: this.activeServerId,
      rooms: { ...this.#rooms },
      order: [...this.#order]
    });
  }
  /** Save zoom before changing geometry; a failed write leaves the current scale intact. */
  setUiScale(value: number): void {
    if (!this.#state) return;
    const next = normalizeScale(value);
    try {
      this.#state.setUi({ ...this.#state.ui, uiScale: next });
      changeScale(next);
      applyTheme();
    } catch {
      this.error = 'Could not save UI scale';
    }
  }
  /** Keep room selection separate for each server and clear the visible draft. */
  select(room: string): void {
    this.#rooms = { ...this.#rooms, [this.activeServerId]: room };
    try {
      this.#save();
    } catch {
      this.error = 'Could not save conversation selection';
    }
  }
  /** Switch servers without replacing their SDK clients or credential state. */
  activate(id: string): void {
    const record = this.#client?.registry.getServer(id);
    if (!record) return;
    this.cancelSignIn();
    this.activeServerId = id;
    this.origin = record.url;
    this.adding = false;
    this.managing = false;
    this.error = '';
    this.draftGeneration++;
    this.#client?.setActiveServer(id);
    try {
      this.#save();
    } catch {
      this.error = 'Could not save server selection';
    }
  }
  /** Show the add-server form while existing servers stay connected. */
  beginAdd(): void {
    this.cancelSignIn();
    this.adding = true;
    this.managing = false;
    this.origin = '';
    this.login = '';
    this.error = '';
    this.draftGeneration++;
  }
  /** Cancel the one in-flight login without discarding registered servers. */
  cancelSignIn(): void {
    this.#epoch++;
    this.#operation?.abort();
    this.#operation = null;
    this.busy = false;
  }

  async #authenticate(browser: boolean): Promise<void> {
    await this.#ready;
    if (this.busy || this.#closed || !this.#client) return;
    this.busy = true;
    this.error = '';
    const epoch = ++this.#epoch;
    const controller = new AbortController();
    this.#operation = controller;
    let issued: { origin: string; credentials: NewBearerSession } | null = null;
    let committing = false;
    try {
      const origin = nativeServerOrigin(this.origin).origin;
      let credentials: NewBearerSession;
      let info: PublicServerInfo;
      if (browser) {
        const result = await (this.#options.authorize ?? browserSignIn)(
          origin,
          this.#options.clientId ??
            process.env.CHATTO_GPUIX_OAUTH_CLIENT_ID ??
            NATIVE_OAUTH_CLIENT_ID,
          controller.signal
        );
        credentials = result.credentials;
        info = result.info;
      } else {
        const password = await (this.#options.password ?? promptPassword)(controller.signal);
        if (epoch !== this.#epoch || password === null) return;
        credentials = await passwordLogin(
          origin,
          { login: this.login, password },
          { signal: controller.signal }
        );
        issued = { origin, credentials };
        info = await getPublicServerInfo(origin, { signal: controller.signal });
      }
      issued = { origin, credentials };
      if (epoch !== this.#epoch || this.#closed || !this.#client) {
        await revokeIssued(origin, credentials);
        issued = null;
        return;
      }
      const registry = this.#client.registry;
      const existing = registry.servers.find((server) => server.url === origin);
      const id = existing?.id ?? `gpuix-${crypto.randomUUID()}`;
      const session = { ...emptyServerSession(), ...persistedBearerSession(credentials) };
      committing = true;
      if (existing) {
        registry.replaceServerAuthentication(id, session);
        registry.updateRegistration(id, { name: info.name, iconUrl: info.iconUrl });
      } else {
        registry.addServer(
          { id, url: origin, name: info.name, iconUrl: info.iconUrl, addedAt: Date.now() },
          session,
          { serverInfo: info }
        );
      }
      issued = null;
      this.#rooms = { ...this.#rooms, [id]: '' };
      this.activate(id);
    } catch {
      if (issued) await revokeIssued(issued.origin, issued.credentials);
      if (committing) {
        // A failed durable write can leave the in-memory registration ahead of
        // disk. Stop renewal until the host reopens the authoritative vault.
        this.#dispose?.();
        this.#dispose = null;
        this.#client?.close();
        this.#client = null;
        const state = this.#state;
        this.#state = null;
        await state?.close();
        this.draftGeneration++;
      }
      if (epoch === this.#epoch)
        this.error = browser
          ? 'Browser sign-in could not be completed. Check the server and OAuth client configuration, then try again.'
          : 'Could not sign in. Check the server and credentials.';
    } finally {
      if (epoch === this.#epoch) {
        this.busy = false;
        this.#operation = null;
      }
    }
  }
  /** Alternate direct-password flow for servers that permit it. */
  signIn(): Promise<void> {
    return this.#authenticate(false);
  }
  /** Primary flow: the server's browser page owns provider/password/consent UI. */
  signInWithBrowser(): Promise<void> {
    return this.#authenticate(true);
  }
  /** Save gutter order without changing server registrations or sessions. */
  moveServer(id: string, offset: number): void {
    const ids = this.servers.map((server) => server.id);
    const index = ids.indexOf(id);
    const target = index + offset;
    if (index < 0 || target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    this.#order = ids;
    try {
      this.#save();
    } catch {
      this.error = 'Could not save server order';
    }
  }
  /** Revoke only the active server's session; leave the server in the gutter for reauthentication. */
  async signOut(): Promise<void> {
    const record = this.activeRegistration;
    if (!record || !this.#client) return;
    this.cancelSignIn();
    this.error = '';
    try {
      this.#client.registry.clearServerAuthentication(record.id);
      this.draftGeneration++;
      await signOutServer(record, false);
    } catch {
      this.error = 'Sign-out could not be completed. Check saved session access and try again.';
    }
  }
  /** Clear local credentials and private stores before requesting remote revocation. */
  async removeServer(id: string): Promise<void> {
    const record = this.#client?.registry.getServer(id);
    if (!record || !this.#client) return;
    this.cancelSignIn();
    try {
      this.#client.registry.removeServer(id);
      const { [id]: removed, ...rooms } = this.#rooms;
      void removed;
      this.#rooms = rooms;
      this.#order = this.#order.filter((entry) => entry !== id);
      if (this.activeServerId === id) {
        this.activeServerId = '';
        const next = this.servers[0];
        if (next) this.activate(next.id);
      }
      this.draftGeneration++;
      this.#save();
      if (record.token) await signOutServer(record, false);
    } catch {
      this.error = 'Local removal or remote session revocation could not be completed.';
    }
  }
  /** Discard decrypted state and network tasks, then release the process lock. */
  close(): Promise<void> {
    if (this.#closing) return this.#closing;
    this.#closed = true;
    this.cancelSignIn();
    this.#dispose?.();
    this.#dispose = null;
    this.#client?.close();
    this.#client = null;
    this.draftGeneration++;
    const state = this.#state;
    this.#state = null;
    this.#closing = state?.close() ?? this.#ready.then(() => {});
    return this.#closing;
  }
}

/** A cancelled or failed handoff must not leave a newly issued session unowned. */
async function revokeIssued(origin: string, credentials: NewBearerSession): Promise<void> {
  const registration: RegisteredServer = {
    ...emptyServerSession(),
    ...persistedBearerSession(credentials),
    id: 'cancelled-native-handoff',
    url: origin,
    name: '',
    iconUrl: null,
    addedAt: 0
  };
  await signOutServer(registration, false).catch(() => {});
}
