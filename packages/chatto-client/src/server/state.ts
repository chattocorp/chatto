/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { batch, signal } from '../reactivity/index.js';
import { getPublicServerInfo, type PublicServerInfo } from '../api/server.js';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { ProjectedServerState } from './projection.js';
import {
  evaluateServerCompatibility,
  isSupportedServerVersion,
  type ServerCompatibilityProblem,
  type ServerCompatibilityResult
} from './compatibility.js';

const DEFAULT_MAX_UPLOAD_SIZE = 25 * 1024 * 1024;
const DEFAULT_MESSAGE_EDIT_WINDOW_SECONDS = 3 * 60 * 60;
/** Name of a server until discovery returns its own name. */
const DEFAULT_SERVER_NAME = 'Chatto';

/**
 * A server's name for display. Until discovery returns a name,
 * `ServerInfoState.name` keeps the default name; `savedName`, for example the
 * name saved at registration, then takes precedence over the default.
 */
export function serverDisplayName(
  serverInfo: Pick<ServerInfoState, 'name'>,
  savedName?: string | null
): string {
  return serverInfo.name !== DEFAULT_SERVER_NAME ? serverInfo.name : savedName || serverInfo.name;
}

export class ServerInfoState {
  #label: string;
  #getPublicServerInfo: (baseUrl: string) => Promise<PublicServerInfo>;
  #initializing: Promise<void> | null = null;

  readonly #nameSignal = signal(DEFAULT_SERVER_NAME);
  get name() {
    return this.#nameSignal.get();
  }
  set name(value) {
    this.#nameSignal.set(value);
  }
  readonly #versionSignal = signal('');
  get version() {
    return this.#versionSignal.get();
  }
  set version(value) {
    this.#versionSignal.set(value);
  }
  readonly #lastDiscoveredAtSignal = signal<number | null>(null);
  get lastDiscoveredAt(): number | null {
    return this.#lastDiscoveredAtSignal.get();
  }
  set lastDiscoveredAt(value: number | null) {
    this.#lastDiscoveredAtSignal.set(value);
  }
  readonly #welcomeMessageSignal = signal<string | null>(null);
  get welcomeMessage(): string | null {
    return this.#welcomeMessageSignal.get();
  }
  set welcomeMessage(value: string | null) {
    this.#welcomeMessageSignal.set(value);
  }
  readonly #descriptionSignal = signal<string | null>(null);
  get description(): string | null {
    return this.#descriptionSignal.get();
  }
  set description(value: string | null) {
    this.#descriptionSignal.set(value);
  }
  readonly #bannerUrlSignal = signal<string | null>(null);
  get bannerUrl(): string | null {
    return this.#bannerUrlSignal.get();
  }
  set bannerUrl(value: string | null) {
    this.#bannerUrlSignal.set(value);
  }
  readonly #iconUrlSignal = signal<string | null>(null);
  get iconUrl(): string | null {
    return this.#iconUrlSignal.get();
  }
  set iconUrl(value: string | null) {
    this.#iconUrlSignal.set(value);
  }
  readonly #directRegistrationEnabledSignal = signal(true);
  get directRegistrationEnabled() {
    return this.#directRegistrationEnabledSignal.get();
  }
  set directRegistrationEnabled(value) {
    this.#directRegistrationEnabledSignal.set(value);
  }
  readonly #directLoginEnabledSignal = signal(true);
  get directLoginEnabled() {
    return this.#directLoginEnabledSignal.get();
  }
  set directLoginEnabled(value) {
    this.#directLoginEnabledSignal.set(value);
  }
  #getProjectedState: () => ProjectedServerState | null;

  readonly #loadingSignal = signal(true);
  get loading() {
    return this.#loadingSignal.get();
  }
  set loading(value) {
    this.#loadingSignal.set(value);
  }

  /**
   * Set when `init()` failed to fetch server info (e.g. unreachable host,
   * CORS misconfiguration). Consumers can use this to render a degraded UI
   * for that server without taking down the rest of the app. Null when the
   * latest load succeeded. A retry keeps the error until the new attempt
   * settles, so the degraded UI stays stable.
   */
  readonly #errorSignal = signal<unknown>(null);
  get error(): unknown {
    return this.#errorSignal.get();
  }
  set error(value: unknown) {
    this.#errorSignal.set(value);
  }

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
   * Why this client cannot use the server, or null. It is null while the first
   * discovery is in flight and when the server is supported. A retry keeps the
   * previous problem until the new attempt settles.
   */
  get compatibilityProblem(): ServerCompatibilityProblem | null {
    const discovered = !this.loading || this.lastDiscoveredAt !== null || this.error !== null;
    if (!discovered) return null;
    const { reason } = this.compatibility;
    return reason === 'version-confirmed' ? null : reason;
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
   * Sets `loading = true` for the duration so consumers can gate their UI,
   * for example a redirect that waits for server metadata.
   */
  async init(): Promise<void> {
    if (this.#initializing) return this.#initializing;

    const initializing = (async () => {
      this.loading = true;
      try {
        await this.refreshProfile();
      } catch (err) {
        // Defensive: anything thrown during the query or above .then body.
        // Don't re-throw — failure is isolated to this server.
        this.error = err;
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
      batch(() => {
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
      });
    } catch (err) {
      this.error = err;
      console.error(`[server:${this.#label}] failed to load server info`, err);
    }
  }

  /** Apply the public profile carried by the realtime projection stream. */
  applyProjectionProfile(profile: ServerPublicProfile): void {
    batch(() => {
      this.name = profile.name;
      this.version = profile.version;
      this.welcomeMessage = profile.welcomeMessage ?? null;
      this.description = profile.description ?? null;
      this.iconUrl = profile.logoUrl ?? null;
      this.bannerUrl = profile.bannerUrl ?? null;
      this.error = null;
      this.loading = false;
    });
  }
}
