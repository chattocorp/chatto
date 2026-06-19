/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { fetchPublicServerInfo } from '$lib/serverInfo';
import { WireClient, type WireClientConfig } from '$lib/wire/client';

type WireSettingsClient = Pick<WireClient, 'getAuthenticatedServerSettings'>;

export class ServerInfoState {
  #wireConfig: WireClientConfig;
  #getWireClient: (() => WireSettingsClient | null | undefined) | null;
  #serverUrl: string;
  #label: string;

  name = $state('Chatto');
  motd = $state<string | null>(null);
  welcomeMessage = $state<string | null>(null);
  description = $state<string | null>(null);
  bannerUrl = $state<string | null>(null);
  iconUrl = $state<string | null>(null);
  directRegistrationEnabled = $state(true);
  pushNotificationsEnabled = $state(false);
  vapidPublicKey = $state<string | null>(null);
  livekitUrl = $state<string | null>(null);
  videoProcessingEnabled = $state(false);
  maxUploadSize = $state(25 * 1024 * 1024); // default 25 MB
  maxVideoUploadSize = $state(25 * 1024 * 1024); // default 25 MB (overridden when video enabled)
  messageEditWindowSeconds = $state(3 * 60 * 60); // default 3 hours; overwritten after auth

  loading = $state(true);

  /**
   * Set when `init()` failed to fetch server info (e.g. unreachable host,
   * CORS misconfiguration). Consumers can use this to render a degraded UI
   * for that server without taking down the rest of the app.
   */
  error = $state<string | null>(null);

  /**
   * Human-readable label for this server, used in log messages so console
   * errors can be traced back to a specific server. Pass the URL (or any
   * stable identifier) — used purely for diagnostics.
   */
  constructor(
    serverUrl = '',
    wireConfig: WireClientConfig = {},
    getWireClient?: () => WireSettingsClient | null | undefined
  ) {
    this.#serverUrl = serverUrl;
    this.#wireConfig = wireConfig;
    this.#getWireClient = getWireClient ?? null;
    this.#label = serverUrl || 'origin';
  }

  /**
   * Fetch server info. Idempotent; can be called again to refresh metadata
   * after live updates.
   *
   * Sets `loading = true` for the duration so consumers can gate their UI
   * (the chat-root page's redirect logic relies on this — see
   * `chat/[serverId]/+page.svelte`).
   */
  async init(): Promise<void> {
    this.loading = true;
    this.error = null;
    try {
      await this.refreshProfile();
    } catch (err) {
      // Defensive: anything thrown during the public server-info fetch.
      // Don't re-throw — failure is isolated to this server.
      this.error = err instanceof Error ? err.message : String(err);
      console.error(`[server:${this.#label}] failed to load server info`, err);
    } finally {
      this.loading = false;
    }
  }

  async refreshProfile(): Promise<void> {
    const info = await fetchPublicServerInfo(this.#serverUrl);
    this.error = null;
    this.name = info.name;
    this.welcomeMessage = info.welcomeMessage;
    this.description = info.description;
    this.iconUrl = info.iconUrl;
    this.bannerUrl = info.bannerUrl;
    this.directRegistrationEnabled = info.directRegistrationEnabled;
  }

  /**
   * Fetch authenticated server settings used by the in-app UI. This runs only
   * after the store knows the viewer is authenticated.
   */
  async refreshAuthenticatedSettings(): Promise<void> {
    const resp = await this.#withWireClient((client) => client.getAuthenticatedServerSettings());
    const settings = resp.settings;
    if (!settings) return;

    this.motd = settings.motd || null;
    this.pushNotificationsEnabled = settings.pushNotificationsEnabled;
    this.vapidPublicKey = settings.vapidPublicKey || null;
    this.livekitUrl = settings.livekitUrl || null;
    this.videoProcessingEnabled = settings.videoProcessingEnabled;
    this.maxUploadSize = Number(settings.maxUploadSize);
    this.maxVideoUploadSize = Number(settings.maxVideoUploadSize);
    this.messageEditWindowSeconds = settings.messageEditWindowSeconds;
  }

  async #withWireClient<T>(task: (client: WireSettingsClient) => Promise<T>): Promise<T> {
    const shared = this.#getWireClient?.();
    if (shared) {
      return task(shared);
    }

    const client = new WireClient(this.#wireConfig);
    try {
      return await task(client);
    } finally {
      client.dispose();
    }
  }
}
