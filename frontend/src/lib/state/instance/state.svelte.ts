/**
 * Instance state - stores instance-wide configuration like name, MOTD, etc.
 * This information is available without authentication.
 */

import { graphql } from '$lib/gql';
import type { Client } from '@urql/svelte';

export class InstanceState {
  #client: Client;
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
  maxUploadSize = $state(25 * 1024 * 1024); // default 25 MB
  maxVideoUploadSize = $state(25 * 1024 * 1024); // default 25 MB (overridden when video enabled)

  /**
   * ID of the space this deployment treats as its primary (future Server).
   * Empty string until the GetInstanceInfo query resolves, or on fresh
   * installs with no user-facing space yet. ADR-027 / #330 migration bridge.
   */
  primarySpaceId = $state('');

  loading = $state(true);

  /**
   * Set when `init()` failed to fetch instance info (e.g. unreachable host,
   * CORS misconfiguration). Consumers can use this to render a degraded UI
   * for that instance without taking down the rest of the app.
   */
  error = $state<string | null>(null);

  /**
   * Human-readable label for this instance, used in log messages so console
   * errors can be traced back to a specific instance. Pass the URL (or any
   * stable identifier) — used purely for diagnostics.
   */
  constructor(client: Client, label = 'unknown') {
    this.#client = client;
    this.#label = label;
  }

  /**
   * Fetch instance info from the server. Idempotent; can be called again to
   * refresh fields like `primarySpaceId` after mutations that may have
   * changed them (e.g. createSpace on an empty instance).
   *
   * Sets `loading = true` for the duration so consumers can gate their UI
   * (the chat-root page's redirect logic relies on this — see
   * `(chrome)/+page.svelte`).
   */
  async init(): Promise<void> {
    this.loading = true;
    this.error = null;
    try {
      const resp = await this.#client
        .query(
          graphql(`
          query GetInstanceInfo {
            server {
              directRegistrationEnabled
              pushNotificationsEnabled
              vapidPublicKey
              livekitUrl
              maxUploadSize
              maxVideoUploadSize
              primarySpaceId
              config {
                serverName
                motd
                welcomeMessage
                description
                logoUrl(width: 256, height: 256)
                bannerUrl(width: 1200, height: 630)
              }
            }
          }
        `),
          {},
          { requestPolicy: 'network-only' }
        )
        .toPromise();

      if (resp.error) {
        // urql surfaces network failures (CORS, DNS, server down) as
        // result.error.networkError rather than rejecting. Treat as a
        // soft per-instance failure: log, set error state, and bail.
        this.error = resp.error.message;
        console.error(
          `[instance:${this.#label}] failed to load instance info`,
          resp.error
        );
        return;
      }

      if (resp.data?.server) {
        this.name = resp.data.server.config.serverName;
        this.motd = resp.data.server.config.motd ?? null;
        this.welcomeMessage = resp.data.server.config.welcomeMessage ?? null;
        this.description = resp.data.server.config.description ?? null;
        this.iconUrl = resp.data.server.config.logoUrl ?? null;
        this.bannerUrl = resp.data.server.config.bannerUrl ?? null;
        this.directRegistrationEnabled = resp.data.server.directRegistrationEnabled;
        this.pushNotificationsEnabled = resp.data.server.pushNotificationsEnabled;
        this.vapidPublicKey = resp.data.server.vapidPublicKey ?? null;
        this.livekitUrl = resp.data.server.livekitUrl ?? null;
        this.maxUploadSize = resp.data.server.maxUploadSize;
        this.maxVideoUploadSize = resp.data.server.maxVideoUploadSize;
        this.primarySpaceId = resp.data.server.primarySpaceId;
      }
    } catch (err) {
      // Defensive: anything thrown during the query or above .then body.
      // Don't re-throw — failure is isolated to this instance.
      this.error = err instanceof Error ? err.message : String(err);
      console.error(
        `[instance:${this.#label}] failed to load instance info`,
        err
      );
    } finally {
      this.loading = false;
    }
  }

  /**
   * Update instance config from a live event.
   * Called when an ServerConfigUpdatedEvent is received.
   */
  updateConfig(config: {
    serverName: string;
    motd: string | null;
    welcomeMessage: string | null;
    description?: string | null;
  }): void {
    this.name = config.serverName;
    this.motd = config.motd;
    this.welcomeMessage = config.welcomeMessage;
    if ('description' in config) {
      this.description = config.description ?? null;
    }
  }
}
