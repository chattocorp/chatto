/**
 * Server info state — public branding plus authenticated runtime settings.
 */

import { graphql } from '$lib/gql';
import type { Client } from '@urql/svelte';

export class ServerInfoState {
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
  constructor(client: Client, label = 'unknown') {
    this.#client = client;
    this.#label = label;
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
      console.error(
        `[server:${this.#label}] failed to load server info`,
        err
      );
    } finally {
      this.loading = false;
    }
  }

  async refreshProfile(): Promise<void> {
    const resp = await this.#client
      .query(
        graphql(`
          query GetServerInfo {
            server {
              directRegistrationEnabled
              profile {
                name
                welcomeMessage
                description
                logoUrl
                bannerUrl
              }
            }
          }
        `),
        {},
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    if (resp.error) {
      this.error = resp.error.message;
      console.error(
        `[server:${this.#label}] failed to load server info`,
        resp.error
      );
      return;
    }

    if (resp.data?.server) {
      this.error = null;
      this.name = resp.data.server.profile.name;
      this.welcomeMessage = resp.data.server.profile.welcomeMessage ?? null;
      this.description = resp.data.server.profile.description ?? null;
      this.iconUrl = resp.data.server.profile.logoUrl ?? null;
      this.bannerUrl = resp.data.server.profile.bannerUrl ?? null;
      this.directRegistrationEnabled = resp.data.server.directRegistrationEnabled;
    }
  }

  /**
   * Fetch authenticated server settings used by the in-app UI. This runs only
   * after the store knows the viewer is authenticated.
   */
  async refreshAuthenticatedSettings(): Promise<void> {
    const resp = await this.#client
      .query(
        graphql(`
          query GetAuthenticatedServerSettings {
            server {
              pushNotificationsEnabled
              vapidPublicKey
              livekitUrl
              videoProcessingEnabled
              maxUploadSize
              maxVideoUploadSize
              messageEditWindowSeconds
              profile {
                motd
              }
            }
          }
        `),
        {},
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    if (resp.error) {
      throw resp.error;
    }

    if (resp.data?.server) {
      this.motd = resp.data.server.profile.motd ?? null;
      this.pushNotificationsEnabled = resp.data.server.pushNotificationsEnabled;
      this.vapidPublicKey = resp.data.server.vapidPublicKey ?? null;
      this.livekitUrl = resp.data.server.livekitUrl ?? null;
      this.videoProcessingEnabled = resp.data.server.videoProcessingEnabled;
      this.maxUploadSize = resp.data.server.maxUploadSize;
      this.maxVideoUploadSize = resp.data.server.maxVideoUploadSize;
      this.messageEditWindowSeconds = resp.data.server.messageEditWindowSeconds;
    }
  }

}
