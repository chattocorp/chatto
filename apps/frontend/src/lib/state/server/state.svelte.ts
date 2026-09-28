/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { errorMessage } from '$lib/utils/errorMessage';
import { getPublicServerInfo, type PublicServerInfo } from '$lib/api-client/server';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { ProjectedServerState } from './projection.svelte';
import {
  evaluateServerCompatibility,
  isSupportedServerVersion,
  type ServerCompatibilityResult
} from './compatibility';

const DEFAULT_MAX_UPLOAD_SIZE = 25 * 1024 * 1024;
const DEFAULT_MESSAGE_EDIT_WINDOW_SECONDS = 3 * 60 * 60;

export class ServerInfoState {
  #label: string;
  #getPublicServerInfo: (baseUrl: string) => Promise<PublicServerInfo>;
  #initializing: Promise<void> | null = null;

  name = $state('Chatto');
  version = $state('');
  lastDiscoveredAt = $state<number | null>(null);
  welcomeMessage = $state<string | null>(null);
  description = $state<string | null>(null);
  bannerUrl = $state<string | null>(null);
  iconUrl = $state<string | null>(null);
  directRegistrationEnabled = $state(true);
  directLoginEnabled = $state(true);
  #getProjectedState: () => ProjectedServerState | null;

  loading = $state(true);

  /**
   * Set when `init()` failed to fetch server info (e.g. unreachable host,
   * CORS misconfiguration). Consumers can use this to render a degraded UI
   * for that server without taking down the rest of the app.
   */
  error = $state<string | null>(null);

  // Authenticated runtime settings read the realtime projection directly, so a
  // projection reset also resets them. Defaults apply until the projection has them.

  /** Message of the day. */
  get motd(): string | null {
    return this.#getProjectedState()?.motd ?? null;
  }

  /** Whether the server sends Web Push notifications. */
  get pushNotificationsEnabled(): boolean {
    return this.#runtime?.pushNotificationsEnabled ?? false;
  }

  /** Public VAPID key for Web Push subscriptions. */
  get vapidPublicKey(): string | null {
    return this.#runtime?.vapidPublicKey ?? null;
  }

  /** LiveKit URL for voice and video calls, or null when calls are not set up. */
  get livekitUrl(): string | null {
    return this.#runtime?.livekitUrl ?? null;
  }

  /** Whether the server accepts video uploads for processing. */
  get videoProcessingEnabled(): boolean {
    return this.#runtime?.videoProcessingEnabled ?? false;
  }

  /** Largest upload in bytes. Default: 25 MB. */
  get maxUploadSize(): number {
    const runtime = this.#runtime;
    return runtime ? Number(runtime.maxUploadSize) : DEFAULT_MAX_UPLOAD_SIZE;
  }

  /** Largest video upload in bytes. Default: 25 MB. */
  get maxVideoUploadSize(): number {
    const runtime = this.#runtime;
    return runtime ? Number(runtime.maxVideoUploadSize) : DEFAULT_MAX_UPLOAD_SIZE;
  }

  /** How long after posting a message can be edited. Default: 3 hours. */
  get messageEditWindowSeconds(): number {
    return this.#runtime?.messageEditWindowSeconds ?? DEFAULT_MESSAGE_EDIT_WINDOW_SECONDS;
  }

  get #runtime() {
    return this.#getProjectedState()?.runtime;
  }

  get compatibility(): ServerCompatibilityResult {
    return evaluateServerCompatibility({
      serverVersion: this.version,
      unreachable: this.error !== null
    });
  }

  /**
   * Whether discovery confirmed a server release that this client supports.
   * It stays false until the version is known. The realtime projection and
   * every client feature require it.
   */
  get isSupportedVersion(): boolean {
    return isSupportedServerVersion(this.version);
  }

  /**
   * Human-readable label for this server, used in log messages so console
   * errors can be traced back to a specific server. Pass the URL (or any
   * stable identifier) — used purely for diagnostics.
   */
  constructor(
    label = 'unknown',
    publicServerInfoLoader = getPublicServerInfo,
    getProjectedState: () => ProjectedServerState | null = () => null
  ) {
    this.#label = label;
    this.#getPublicServerInfo = publicServerInfoLoader;
    this.#getProjectedState = getProjectedState;
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
    if (this.#initializing) return this.#initializing;

    const initializing = (async () => {
      this.loading = true;
      this.error = null;
      try {
        await this.refreshProfile();
      } catch (err) {
        // Defensive: anything thrown during the query or above .then body.
        // Don't re-throw — failure is isolated to this server.
        this.error = errorMessage(err);
        console.error(`[server:${this.#label}] failed to load server info`, err);
      } finally {
        this.loading = false;
      }
    })();
    this.#initializing = initializing;
    try {
      await initializing;
    } finally {
      if (this.#initializing === initializing) this.#initializing = null;
    }
  }

  async refreshProfile(): Promise<void> {
    try {
      const info = await this.#getPublicServerInfo(this.#label);
      this.error = null;
      this.name = info.name;
      this.version = info.version;
      this.lastDiscoveredAt = Date.now();
      this.welcomeMessage = info.welcomeMessage;
      this.description = info.description;
      this.iconUrl = info.iconUrl;
      this.bannerUrl = info.bannerUrl;
      this.directRegistrationEnabled = info.directRegistrationEnabled;
      this.directLoginEnabled = info.directLoginEnabled;
    } catch (err) {
      this.error = errorMessage(err);
      console.error(`[server:${this.#label}] failed to load server info`, err);
    }
  }

  /** Apply the public profile carried by the realtime projection stream. */
  applyProjectionProfile(profile: ServerPublicProfile): void {
    this.name = profile.name;
    this.version = profile.version;
    this.welcomeMessage = profile.welcomeMessage ?? null;
    this.description = profile.description ?? null;
    this.iconUrl = profile.logoUrl ?? null;
    this.bannerUrl = profile.bannerUrl ?? null;
    this.error = null;
    this.loading = false;
  }
}
