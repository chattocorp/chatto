/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { getPublicServerInfo, type PublicServerInfo } from '$lib/api-client/server';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { RealtimeProjectionServerState } from '@chatto/api-types/realtime/v1/realtime_pb';
import {
  evaluateServerCompatibility,
  hasProtocolCapability,
  REALTIME_PROJECTION_CAPABILITY,
  type ServerCompatibilityResult
} from './compatibility';

export class ServerInfoState {
  #label: string;
  #getPublicServerInfo: (baseUrl: string) => Promise<PublicServerInfo>;
  #initializing: Promise<void> | null = null;

  name = $state('Chatto');
  version = $state('');
  protocolCapabilities = $state<string[] | null>(null);
  minimumWebClientVersion = $state<string | null>(null);
  lastDiscoveredAt = $state<number | null>(null);
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

  get compatibility(): ServerCompatibilityResult {
    return evaluateServerCompatibility({
      serverVersion: this.version,
      protocolCapabilities: this.protocolCapabilities,
      minimumWebClientVersion: this.minimumWebClientVersion,
      unreachable: this.error !== null
    });
  }

  supportsProtocolCapability(capability: string): boolean | null {
    return hasProtocolCapability(this.protocolCapabilities, capability);
  }

  /** Whether discovery confirmed the projection stream required by this client. */
  get supportsRealtimeProjection(): boolean {
    return this.supportsProtocolCapability(REALTIME_PROJECTION_CAPABILITY) === true;
  }

  /**
   * Human-readable label for this server, used in log messages so console
   * errors can be traced back to a specific server. Pass the URL (or any
   * stable identifier) — used purely for diagnostics.
   */
  constructor(label = 'unknown', publicServerInfoLoader = getPublicServerInfo) {
    this.#label = label;
    this.#getPublicServerInfo = publicServerInfoLoader;
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
        this.error = err instanceof Error ? err.message : String(err);
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
      this.protocolCapabilities = info.compatibility?.protocolCapabilities ?? null;
      this.minimumWebClientVersion = info.compatibility?.minimumWebClientVersion ?? null;
      this.lastDiscoveredAt = Date.now();
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

  /** Apply authenticated runtime state carried by the realtime projection. */
  applyProjectionState(state: RealtimeProjectionServerState): void {
    this.motd = state.motd ?? null;
    const runtime = state.runtime;
    if (!runtime) return;
    this.pushNotificationsEnabled = runtime.pushNotificationsEnabled;
    this.vapidPublicKey = runtime.vapidPublicKey ?? null;
    this.livekitUrl = runtime.livekitUrl ?? null;
    this.videoProcessingEnabled = runtime.videoProcessingEnabled;
    this.maxUploadSize = Number(runtime.maxUploadSize);
    this.maxVideoUploadSize = Number(runtime.maxVideoUploadSize);
    this.messageEditWindowSeconds = runtime.messageEditWindowSeconds;
  }

  /**
   * Clear authenticated projection state while preserving independently
   * discovered public profile and protocol-compatibility information.
   */
  resetProjectionState(): void {
    this.motd = null;
    this.pushNotificationsEnabled = false;
    this.vapidPublicKey = null;
    this.livekitUrl = null;
    this.videoProcessingEnabled = false;
    this.maxUploadSize = 25 * 1024 * 1024;
    this.maxVideoUploadSize = 25 * 1024 * 1024;
    this.messageEditWindowSeconds = 3 * 60 * 60;
  }
}
