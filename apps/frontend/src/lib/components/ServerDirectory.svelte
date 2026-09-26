<!--
@component

The Server Directory: a direct server-address lookup and the merged cached
Neighborhoods of all registered servers. It renders without panel chrome so the
Add Server dialog and the `/chat/servers` page can both host it. See FDR-042.
-->
<script lang="ts">
  import { ConnectError } from '@connectrpc/connect';
  import { onMount } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import {
    getPublicServerInfo,
    InvalidPublicServerError,
    type NeighborhoodServerProfile,
    type PublicServerInfo
  } from '$lib/api-client/server';
  import {
    startRemoteReauthentication,
    startServerOAuthFlow,
    startServerOAuthFlowWhenReady
  } from '$lib/auth/reauth';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    canonicalServerOrigin,
    loadServerDirectory,
    type ServerDirectory,
    type ServerDirectoryEntry
  } from '$lib/serverDirectory';
  import { evaluateServerCompatibility } from '$lib/state/server/compatibility';
  import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';
  import { EmptyState, Hint, LoadingFog } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let {
    inDialog = false
  }: {
    /**
     * The directory is the content of the history-backed Add Server dialog.
     * Opening a registered server then replaces the dialog's history entry,
     * so Back does not reopen the dialog.
     */
    inDialog?: boolean;
  } = $props();

  /** A live profile from a direct lookup, or a cached Server Directory profile. */
  type ServerVersionProfile = PublicServerInfo | NeighborhoodServerProfile | null;

  /** The current profile cannot start a sign-in from this client. */
  class ServerJoinUnavailableError extends Error {}

  const headingTag = $derived(inDialog ? 'h3' : 'h2');

  let customInput = $state('');
  let customOrigin = $state('');
  let customProfile = $state<PublicServerInfo | null>(null);
  let customError = $state('');
  let probing = $state(false);
  let pendingOrigin = $state<string | null>(null);
  let directory = $state<ServerDirectory | null>(null);
  let directoryController: AbortController | null = null;
  /** Current profiles that a join action loaded; they replace cached profiles. */
  const liveProfiles = new SvelteMap<string, PublicServerInfo>();

  const registeredOrigins = $derived.by(() => [
    ...new Set(
      serverRegistry.servers.flatMap((server) => {
        const origin = canonicalServerOrigin(server.url);
        return origin ? [origin] : [];
      })
    )
  ]);
  const entries = $derived(directory?.entries ?? []);
  const recommendedEntries = $derived(entries.filter((entry) => !registeredServer(entry.origin)));
  const joinedEntries = $derived(entries.filter((entry) => registeredServer(entry.origin)));
  const allSourcesFailed = $derived(
    !!directory &&
      directory.sourceCount > 0 &&
      directory.failedSourceCount === directory.sourceCount
  );
  const someSourcesFailed = $derived(
    !!directory && directory.failedSourceCount > 0 && !allSourcesFailed
  );

  onMount(() => {
    if (registeredOrigins.length > 0) void refreshDirectory();
    return () => directoryController?.abort();
  });

  /** Load the cached Neighborhood of each registered server. */
  async function refreshDirectory() {
    directoryController?.abort();
    const controller = new AbortController();
    directoryController = controller;
    directory = null;
    try {
      const loaded = await loadServerDirectory(registeredOrigins, { signal: controller.signal });
      if (!controller.signal.aborted) directory = loaded;
    } catch {
      // Closing the directory aborts the request; no state remains to update.
    }
  }

  function normalizeCustomInput(value: string): string {
    const trimmed = value.trim();
    return /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
  }

  function hasScheme(value: string): boolean {
    return /^https?:\/\//i.test(value.trim());
  }

  async function probeCustomServer() {
    customError = '';
    customProfile = null;
    customOrigin = '';
    const initialOrigin = canonicalServerOrigin(normalizeCustomInput(customInput));
    if (!initialOrigin) {
      customError = m('add_server.invalid_url');
      return;
    }

    probing = true;
    try {
      let origin = initialOrigin;
      let profile: PublicServerInfo;
      try {
        profile = await getPublicServerInfo(origin, { signal: AbortSignal.timeout(10_000) });
      } catch (error) {
        if (hasScheme(customInput) || !origin.startsWith('https://')) throw error;
        origin = `http://${origin.slice('https://'.length)}`;
        profile = await getPublicServerInfo(origin, { signal: AbortSignal.timeout(10_000) });
      }
      customOrigin = origin;
      customProfile = profile;
    } catch (error) {
      customError = discoveryError(error);
    } finally {
      probing = false;
    }
  }

  function registeredServer(origin: string): RegisteredServer | undefined {
    return serverRegistry.servers.find(
      (server) => canonicalServerOrigin(server.url) === canonicalServerOrigin(origin)
    );
  }

  function actionLabel(origin: string, profile: ServerVersionProfile): string {
    const joined = registeredServer(origin);
    if (joined) {
      return serverRegistry.isAuthenticated(joined.id)
        ? m('add_server.directory.open')
        : m('add_server.sign_in');
    }
    if (opensInServerClient(origin, profile)) return m('add_server.directory.open_in_new_tab');
    if (isPublicServerInfo(profile) && !profile.authorizeUrl) {
      return m('add_server.directory.sign_in_unavailable');
    }
    return m('add_server.directory.join');
  }

  /** Load the current profile for a join and keep it for the card. */
  async function loadJoinableProfile(origin: string): Promise<PublicServerInfo> {
    const profile = await getPublicServerInfo(origin, { signal: AbortSignal.timeout(10_000) });
    liveProfiles.set(origin, profile);
    if (!canJoin(profile) || opensInServerClient(origin, profile)) {
      throw new ServerJoinUnavailableError();
    }
    return profile;
  }

  function canJoin(profile: ServerVersionProfile): boolean {
    return profile !== null && (!isPublicServerInfo(profile) || !!profile.authorizeUrl);
  }

  function isPublicServerInfo(profile: ServerVersionProfile): profile is PublicServerInfo {
    return profile !== null && 'authorizeUrl' in profile;
  }

  function opensInServerClient(origin: string, profile: ServerVersionProfile): boolean {
    return (
      !registeredServer(origin) &&
      profile !== null &&
      evaluateServerCompatibility({ serverVersion: profile.version }).status !== 'supported'
    );
  }

  /**
   * Open a registered server or start joining a new one. A Server Directory
   * result has only the cached profile, so joining first loads the server's
   * current sign-in data. The user starts this request explicitly.
   */
  async function openOrJoin(origin: string, profile: ServerVersionProfile) {
    const joined = registeredServer(origin);
    if (!joined && !canJoin(profile)) return;
    pendingOrigin = origin;
    try {
      if (joined && serverRegistry.isAuthenticated(joined.id)) {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(joined.id) }), {
          replaceState: inDialog
        });
      } else if (joined) {
        await startRemoteReauthentication(joined);
      } else if (isPublicServerInfo(profile)) {
        await startServerOAuthFlow(origin, profile);
      } else if (profile) {
        // The sign-in window must open from this click, before the current
        // profile loads. A stale cached profile can hide an incompatible
        // version or missing sign-in support.
        await startServerOAuthFlowWhenReady(origin, loadJoinableProfile(origin));
      }
    } catch (error) {
      toast.error(
        error instanceof ServerJoinUnavailableError
          ? m('add_server.directory.sign_in_unavailable')
          : m('add_server.start_failed')
      );
    } finally {
      pendingOrigin = null;
    }
  }

  function discoveryError(error: unknown): string {
    if (
      error instanceof DOMException &&
      (error.name === 'AbortError' || error.name === 'TimeoutError')
    ) {
      return m('add_server.connection_timed_out');
    }
    if (error instanceof InvalidPublicServerError) return m('add_server.not_chatto_server');
    if (error instanceof TypeError || error instanceof ConnectError) {
      return m('add_server.connection_failed');
    }
    return error instanceof Error ? error.message : m('add_server.connect_failed');
  }

  function hostOf(origin: string): string {
    try {
      return new URL(origin).host;
    } catch {
      return origin;
    }
  }

  function sourceName(origin: string): string {
    const registered = registeredServer(origin);
    if (registered) return registered.name;
    const discovered = entries.find((entry) => entry.origin === origin);
    return discovered?.profile?.name || hostOf(origin);
  }

  function sourceAttribution(entry: ServerDirectoryEntry): { visible: string; full: string } {
    const names = entry.sourceOrigins.map(sourceName);
    const formatter = new Intl.ListFormat(getReactiveLocale(), {
      style: 'long',
      type: 'conjunction'
    });
    const remaining = names.length - 2;
    const visibleNames =
      remaining > 0
        ? [
            ...names.slice(0, 2),
            m('add_server.directory.more_recommenders_count', { count: remaining })
          ]
        : names;
    return {
      visible: m('add_server.directory.recommended_by', {
        servers: formatter.format(visibleNames)
      }),
      full: m('add_server.directory.recommended_by', { servers: formatter.format(names) })
    };
  }
</script>

{#snippet entryAction(origin: string, profile: ServerVersionProfile, fullWidth: boolean)}
  {@const joined = registeredServer(origin)}
  {#if opensInServerClient(origin, profile)}
    <Button href={origin} opensInNewTab variant="secondary" size="sm" {fullWidth}>
      <span>{actionLabel(origin, profile)}</span>
      <span class="iconify icon-[uil--external-link-alt]" aria-hidden="true"></span>
    </Button>
  {:else}
    <Button
      variant={joined ? 'secondary' : 'action'}
      size="sm"
      {fullWidth}
      loading={pendingOrigin === origin}
      disabled={!joined && !canJoin(profile)}
      onclick={() => openOrJoin(origin, profile)}
    >
      {actionLabel(origin, profile)}
    </Button>
  {/if}
{/snippet}

<div class="flex flex-col gap-8">
  <section
    class="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5"
    aria-labelledby="server-directory-lookup-title"
  >
    <div class="flex flex-col gap-1">
      <svelte:element
        this={headingTag}
        id="server-directory-lookup-title"
        class="text-lg font-semibold text-balance text-text-top"
      >
        {m('add_server.directory.custom_title')}
      </svelte:element>
      <p class="text-sm text-pretty text-muted">
        {m('add_server.directory.lookup_description')}
      </p>
    </div>

    <Form onsubmit={probeCustomServer} error={customError}>
      <div class="flex flex-col items-stretch gap-3 sm:flex-row">
        <div class="min-w-0 flex-1">
          <TextInput
            id="add-server-url"
            label={m('add_server.url_label')}
            labelHidden
            bind:value={customInput}
            placeholder={m('add_server.url_placeholder')}
            leadingIcon="icon-[uil--globe]"
            disabled={probing}
            required
          />
        </div>
        <Button
          type="submit"
          loading={probing}
          loadingText={m('add_server.connecting')}
          disabled={!customInput.trim()}
        >
          {m('add_server.directory.find')}
        </Button>
      </div>
    </Form>

    {#if customProfile && customOrigin}
      {@const profile = customProfile}
      {@const joined = registeredServer(customOrigin)}
      {@const external = opensInServerClient(customOrigin, profile)}
      <div class="max-w-md">
        {#snippet customActions()}
          {@render entryAction(customOrigin, profile, true)}
        {/snippet}
        <ServerProfileCard
          origin={customOrigin}
          {profile}
          badge={joined ? m('add_server.directory.joined') : undefined}
          iconHref={external ? customOrigin : undefined}
          iconOpensInNewTab={external}
          onIconClick={external || (!joined && !canJoin(profile))
            ? undefined
            : () => openOrJoin(customOrigin, profile)}
          iconActionLabel={actionLabel(customOrigin, profile)}
          iconActionDisabled={pendingOrigin === customOrigin}
          actions={customActions}
          testId="server-directory-entry"
          headingTag={inDialog ? 'h4' : 'h3'}
        />
      </div>
    {/if}
  </section>

  {#if registeredOrigins.length > 0}
    <section class="flex flex-col gap-4" aria-labelledby="server-directory-recommended-title">
      <div class="flex flex-col gap-1">
        <svelte:element
          this={headingTag}
          id="server-directory-recommended-title"
          class="text-lg font-semibold text-balance text-text-top"
        >
          {m('add_server.directory.servers_title')}
          {#if recommendedEntries.length > 0}
            <span class="font-normal text-muted tabular-nums">({recommendedEntries.length})</span>
          {/if}
        </svelte:element>
        <p class="text-sm text-pretty text-muted">
          {m('add_server.directory.servers_description')}
        </p>
      </div>

      {#if someSourcesFailed}
        <Hint tone="warning">{m('add_server.directory.partial')}</Hint>
      {/if}

      {#if !directory}
        <LoadingFog class="h-64 w-full rounded-lg" />
      {:else if allSourcesFailed}
        <EmptyState
          icon="icon-[uil--exclamation-triangle]"
          title={m('add_server.directory.unavailable_title')}
        >
          <div class="flex flex-col items-center gap-3">
            <span>{m('add_server.directory.unavailable_body')}</span>
            <Button variant="secondary" onclick={refreshDirectory}>
              {m('common.retry')}
            </Button>
          </div>
        </EmptyState>
      {:else if recommendedEntries.length === 0}
        <EmptyState icon="icon-[uil--compass]" title={m('add_server.directory.empty_title')}>
          {m('add_server.directory.empty_body')}
        </EmptyState>
      {:else}
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {#each recommendedEntries as entry (entry.origin)}
            {@const profile = liveProfiles.get(entry.origin) ?? entry.profile}
            {@const external = opensInServerClient(entry.origin, profile)}
            {@const attribution = sourceAttribution(entry)}
            {#snippet cardActions()}
              <div class="flex items-center gap-3">
                <p
                  class="line-clamp-2 min-w-0 flex-1 text-sm text-muted"
                  aria-label={attribution.full}
                  title={attribution.full}
                  data-testid="server-recommendation-sources"
                >
                  <bdi>{attribution.visible}</bdi>
                </p>
                {@render entryAction(entry.origin, profile, false)}
              </div>
            {/snippet}
            <ServerProfileCard
              origin={entry.origin}
              imageOrigin={entry.imageOrigin}
              profile={entry.profile}
              iconHref={external ? entry.origin : undefined}
              iconOpensInNewTab={external}
              onIconClick={external || !canJoin(profile)
                ? undefined
                : () => openOrJoin(entry.origin, profile)}
              iconActionLabel={actionLabel(entry.origin, profile)}
              iconActionDisabled={pendingOrigin === entry.origin}
              actions={cardActions}
              testId="server-directory-entry"
              headingTag={inDialog ? 'h4' : 'h3'}
            />
          {/each}
        </div>
      {/if}

      {#if joinedEntries.length > 0}
        <div class="flex flex-col gap-2" data-testid="server-directory-joined">
          <svelte:element this={inDialog ? 'h4' : 'h3'} class="text-sm font-semibold text-muted">
            {m('add_server.directory.joined_title')}
          </svelte:element>
          <ul class="selectable-list">
            {#each joinedEntries as entry (entry.origin)}
              <li
                class="flex items-center gap-3 selectable-list-item px-2 py-1.5"
                data-testid="server-directory-entry"
                data-origin={entry.origin}
              >
                <div class="h-8 w-8 shrink-0 overflow-hidden rounded-md">
                  <ServerLogo
                    server={{ name: entry.profile.name, logoUrl: entry.profile.iconUrl }}
                    publicImageOrigin={entry.imageOrigin}
                    fill
                  />
                </div>
                <div class="flex min-w-0 flex-1 items-baseline gap-2 text-sm">
                  <span class="truncate font-medium text-text-top">
                    <bdi dir="auto">{entry.profile.name}</bdi>
                  </span>
                  <span class="truncate text-muted" dir="ltr">{hostOf(entry.origin)}</span>
                </div>
                {@render entryAction(entry.origin, entry.profile, false)}
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    </section>
  {/if}
</div>
