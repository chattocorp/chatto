/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { getPublicServerInfo, type PublicServerInfo } from '$lib/api-client/server';
import {
  getAuthenticatedServerState,
  type AuthenticatedServerState,
  type ServerStateAPIConfig
} from '$lib/api-client/serverState';

export class ServerInfoState {
  #label: string;
  #getPublicServerInfo: (baseUrl: string) => Promise<PublicServerInfo>;
  #apiConfig?: ServerStateAPIConfig;
  #getAuthenticatedServerState: (config: ServerStateAPIConfig) => Promise<AuthenticatedServerState>;

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
    label = 'unknown',
    publicServerInfoLoader = getPublicServerInfo,
    apiConfig?: ServerStateAPIConfig,
    authenticatedServerStateLoader = getAuthenticatedServerState
  ) {
    this.#label = label;
    this.#getPublicServerInfo = publicServerInfoLoader;
    this.#apiConfig = apiConfig;
    this.#getAuthenticatedServerState = authenticatedServerStateLoader;
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
      // Defensive: anything thrown during the query or above .then body.
      // Don't re-throw — failure is isolated to this server.
      this.error = err instanceof Error ? err.message : String(err);
      console.error(`[server:${this.#label}] failed to load server info`, err);
    } finally {
      this.loading = false;
    }
  }

  async refreshProfile(): Promise<void> {
    try {
      const info = await this.#getPublicServerInfo(this.#label);
      this.error = null;
      this.name = info.name;
      this.welcomeMessage = info.welcomeMessage;
      this.description = info.description;
      this.iconUrl = info.iconUrl;
      this.bannerUrl = info.bannerUrl;
      this.directRegistrationEnabled = info.directRegistrationEnabled;
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
      console.error(`[server:${this.#label}] failed to load server info`, err);
    }
  }

  /**
   * Fetch authenticated server settings used by the in-app UI. This runs only
   * after the store knows the viewer is authenticated.
   */
  async refreshAuthenticatedSettings(): Promise<void> {
    if (!this.#apiConfig) {
      throw new Error('authenticated server state Connect API config is not configured');
    }
    const info = await this.#getAuthenticatedServerState(this.#apiConfig);

    this.motd = info.motd;
    this.pushNotificationsEnabled = info.pushNotificationsEnabled;
    this.vapidPublicKey = info.vapidPublicKey;
    this.livekitUrl = info.livekitUrl;
    this.videoProcessingEnabled = info.videoProcessingEnabled;
    this.maxUploadSize = info.maxUploadSize;
    this.maxVideoUploadSize = info.maxVideoUploadSize;
    this.messageEditWindowSeconds = info.messageEditWindowSeconds;
  }
}
