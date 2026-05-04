/**
 * Instance state - stores instance-wide configuration like name, MOTD, etc.
 * This information is available without authentication.
 */

import { graphql } from '$lib/gql';
import type { Client } from '@urql/svelte';

export class InstanceState {
  #client: Client;

  name = $state('Chatto');
  motd = $state<string | null>(null);
  welcomeMessage = $state<string | null>(null);
  ogImageUrl = $state<string | null>(null);
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

  constructor(client: Client) {
    this.#client = client;
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
  init(): void {
    this.loading = true;
    this.#client
      .query(
        graphql(`
          query GetInstanceInfo {
            instance {
              directRegistrationEnabled
              pushNotificationsEnabled
              vapidPublicKey
              livekitUrl
              maxUploadSize
              maxVideoUploadSize
              primarySpaceId
              config {
                instanceName
                motd
                welcomeMessage
                ogImageUrl(width: 768, height: 402)
              }
            }
          }
        `),
        {},
        { requestPolicy: 'network-only' }
      )
      .then((resp) => {
        if (resp.data?.instance) {
          this.name = resp.data.instance.config.instanceName;
          this.motd = resp.data.instance.config.motd ?? null;
          this.welcomeMessage = resp.data.instance.config.welcomeMessage ?? null;
          this.ogImageUrl = resp.data.instance.config.ogImageUrl ?? null;
          this.directRegistrationEnabled = resp.data.instance.directRegistrationEnabled;
          this.pushNotificationsEnabled = resp.data.instance.pushNotificationsEnabled;
          this.vapidPublicKey = resp.data.instance.vapidPublicKey ?? null;
          this.livekitUrl = resp.data.instance.livekitUrl ?? null;
          this.maxUploadSize = resp.data.instance.maxUploadSize;
          this.maxVideoUploadSize = resp.data.instance.maxVideoUploadSize;
          this.primarySpaceId = resp.data.instance.primarySpaceId;
        }
        this.loading = false;
      });
  }

  /**
   * Update instance config from a live event.
   * Called when an InstanceConfigUpdatedEvent is received.
   */
  updateConfig(config: {
    instanceName: string;
    motd: string | null;
    welcomeMessage: string | null;
  }): void {
    this.name = config.instanceName;
    this.motd = config.motd;
    this.welcomeMessage = config.welcomeMessage;
  }
}
