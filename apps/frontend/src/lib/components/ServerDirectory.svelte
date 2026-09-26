<!--
@component

The Server Directory: a direct server-address lookup and the merged cached
Neighborhoods of all registered servers. The `/chat/servers` page shows each
section in a titled panel; the Add Server dialog shows the sections directly on
its work plane. See FDR-042.
-->
<script lang="ts">
  import { ConnectError } from '@connectrpc/connect';
  import { onMount, type Snippet } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
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
    startServerOAuthFlowWhenReady,
    type ServerOAuthFlowOptions
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
  import { EmptyState, Hint, LoadingFog, Panel } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let {
    inDialog = false
  }: {
    /**
     * The directory is the content of the history-backed Add Server dialog.
     * Sections then sit on the dialog's work plane with one lower heading
     * level. Opening, joining, or signing in to a server replaces the
     * dialog's history entry while it is open, so Back does not reopen it.
     */
    inDialog?: boolean;
  } = $props();

  /** A live profile from a direct lookup, or a cached Server Directory profile. */
  type ServerVersionProfile = PublicServerInfo | NeighborhoodServerProfile | null;

  /** The current profile cannot start a sign-in from this client. */
  class ServerJoinUnavailableError extends Error {}

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
   * Sign-in can finish after the dialog closes. Replace history only when the
   * Add Server dialog is still the current entry at that time.
   */
  const signInOptions: ServerOAuthFlowOptions = {
    replaceHistory: () => inDialog && page.state.modal?.type === 'addServer'
  };

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
        await startRemoteReauthentication(joined, signInOptions);
      } else if (isPublicServerInfo(profile)) {
        await startServerOAuthFlow(origin, profile, signInOptions);
      } else if (profile) {
        // The sign-in window must open from this click, before the cached
        // profile is refreshed. A stale cached profile can hide an
        // incompatible version or missing sign-in support.
        await startServerOAuthFlowWhenReady(origin, loadJoinableProfile(origin), signInOptions);
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

{#snippet recommendationSources(entry: ServerDirectoryEntry, className: string)}
  {@const attribution = sourceAttribution(entry)}
  <p
    class={['min-w-0 text-sm text-muted', className]}
    aria-label={attribution.full}
    title={attribution.full}
    data-testid="server-recommendation-sources"
  >
    <bdi>{attribution.visible}</bdi>
  </p>
{/snippet}

<!-- The page frames each section in a titled Panel. The dialog already owns
     one work plane, so its sections sit directly on it. -->
{#snippet directorySection(
  id: string,
  title: string,
  description: string,
  count: number | undefined,
  body: Snippet
)}
  {#if inDialog}
    <section class="flex flex-col gap-4" aria-labelledby={id}>
      <div class="flex flex-col gap-1">
        <h3 {id} class="text-base font-semibold text-balance text-text-top">
          {title}
          {#if count !== undefined}
            <span class="font-normal text-muted tabular-nums">({count})</span>
          {/if}
        </h3>
        <p class="text-sm text-pretty text-muted">{description}</p>
      </div>
      {@render body()}
    </section>
  {:else}
    <Panel {title} subtitle={description} {count}>
      <div class="flex flex-col gap-4">{@render body()}</div>
    </Panel>
  {/if}
{/snippet}

{#snippet lookupBody()}
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
{/snippet}

{#snippet recommendationsBody()}
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
        {#snippet cardActions()}
          <div class="flex items-center gap-3">
            {@render recommendationSources(entry, 'line-clamp-2 flex-1')}
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
      <ul class="flex flex-col gap-1">
        {#each joinedEntries as entry (entry.origin)}
          <li
            class="flex items-center gap-3 rounded-md px-2 py-1.5"
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
            <div class="flex min-w-0 flex-1 flex-col text-sm">
              <div class="flex min-w-0 items-baseline gap-2">
                <span class="truncate font-medium text-text-top">
                  <bdi dir="auto">{entry.profile.name}</bdi>
                </span>
                <span class="truncate text-muted" dir="ltr">{hostOf(entry.origin)}</span>
              </div>
              {@render recommendationSources(entry, 'truncate')}
            </div>
            {@render entryAction(entry.origin, entry.profile, false)}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
{/snippet}

<div class={['flex flex-col', inDialog ? 'gap-8' : 'gap-6']}>
  {@render directorySection(
    'server-directory-lookup-title',
    m('add_server.directory.custom_title'),
    m('add_server.directory.lookup_description'),
    undefined,
    lookupBody
  )}

  {#if registeredOrigins.length > 0}
    {@render directorySection(
      'server-directory-recommended-title',
      m('add_server.directory.servers_title'),
      m('add_server.directory.servers_description'),
      recommendedEntries.length || undefined,
      recommendationsBody
    )}
  {/if}
</div>
