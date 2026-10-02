<!--
@component

The Server Directory: the merged cached Neighborhoods of all registered
servers, and a direct server-address lookup that opens from a button. The
`/chat/servers` page frames the directory in a titled panel; the Add Server
dialog shows it directly on its work plane. See FDR-042.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
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
  } from '@chatto/client/api/server';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    loadServerDirectory,
    type ServerDirectory,
    type ServerDirectoryEntry
  } from '$lib/serverDirectory';
  import { canonicalServerOrigin, serverHost } from '@chatto/client/util/serverUrl';
  import { evaluateServerCompatibility } from '@chatto/client/server/compatibility';
  import { serverRegistry } from '$lib/client';
  import { EmptyState, Hint, LoadingFog, Panel } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let {
    inDialog = false
  }: {
    /**
     * The directory is the content of the history-backed Add Server dialog.
     * Sections then sit on the dialog's work plane with one lower heading
     * level. Opening a server replaces the dialog's history entry, so Back
     * does not reopen it. Joining a server keeps the dialog open.
     */
    inDialog?: boolean;
  } = $props();

  /** A live profile from a direct lookup, or a cached Server Directory profile. */
  type ServerVersionProfile = PublicServerInfo | NeighborhoodServerProfile | null;

  /** The current profile does not let this client sign in. */
  class ServerJoinUnavailableError extends Error {}

  /** The user asked for the direct address lookup. */
  let lookupRequested = $state(false);
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
  const allSourcesFailed = $derived(
    !!directory &&
      directory.sourceCount > 0 &&
      directory.failedSourceCount === directory.sourceCount
  );
  const someSourcesFailed = $derived(
    !!directory && directory.failedSourceCount > 0 && !allSourcesFailed
  );
  /** The lookup shows at once when the directory has nothing to recommend. */
  const showLookup = $derived(lookupRequested || (!!directory && entries.length === 0));

  onMount(() => {
    void refreshDirectory();
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

  function actionLabel(origin: string, profile: ServerVersionProfile): string {
    if (serverRegistry.findServerByUrl(origin)) return m('add_server.directory.open');
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
      !serverRegistry.findServerByUrl(origin) &&
      profile !== null &&
      evaluateServerCompatibility({ serverVersion: profile.version }).status !== 'supported'
    );
  }

  /**
   * Open a registered server, or add a new one to the gutter. Adding a server
   * neither opens it nor starts sign-in, so the user can add more servers
   * before they close the directory. The entry then shows the joined state
   * with an open action. The server's signed-out view offers **Log in to this
   * server**. A Server Directory result has only the cached profile, so adding
   * it first loads the server's current profile. The user starts this request
   * explicitly.
   */
  async function openOrJoin(origin: string, profile: ServerVersionProfile) {
    const joined = serverRegistry.findServerByUrl(origin);
    if (!joined && !canJoin(profile)) return;
    pendingOrigin = origin;
    try {
      if (joined) {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(joined.id) }), {
          replaceState: inDialog
        });
        return;
      }
      let current: PublicServerInfo;
      try {
        // A stale cached profile can hide an incompatible version or missing
        // sign-in support.
        current = isPublicServerInfo(profile) ? profile : await loadJoinableProfile(origin);
      } catch (error) {
        toast.error(
          error instanceof ServerJoinUnavailableError
            ? m('add_server.directory.sign_in_unavailable')
            : discoveryError(error)
        );
        return;
      }
      serverRegistry.addSignedOutServer(origin, {
        name: current.name,
        iconUrl: current.iconUrl ?? null
      });
    } catch {
      toast.error(m('common.error.generic'));
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
    return errorMessage(error, m('add_server.connect_failed'));
  }

  function sourceName(origin: string): string {
    const registered = serverRegistry.findServerByUrl(origin);
    if (registered) return registered.name;
    const discovered = entries.find((entry) => entry.origin === origin);
    if (discovered?.profile?.name) return discovered.profile.name;
    return serverHost(origin);
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
  {@const joined = serverRegistry.findServerByUrl(origin)}
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

{#snippet lookupButton()}
  <Button variant="secondary" size="sm" onclick={() => (lookupRequested = true)}>
    <span class="iconify icon-[uil--globe]" aria-hidden="true"></span>
    <span>{m('add_server.directory.connect_by_address')}</span>
  </Button>
{/snippet}

{#snippet lookupForm()}
  <section
    class="flex flex-col gap-3 surface-box p-4"
    aria-label={m('add_server.directory.connect_by_address')}
    data-testid="server-directory-lookup"
  >
    <p class="text-sm text-pretty text-muted">{m('add_server.directory.lookup_description')}</p>
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
            autofocus={lookupRequested}
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
      {@const joined = serverRegistry.findServerByUrl(customOrigin)}
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
{/snippet}

{#snippet recommendationsBody()}
  {#if showLookup}
    {@render lookupForm()}
  {/if}

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
  {:else if entries.length === 0}
    <EmptyState icon="icon-[uil--compass]" title={m('add_server.directory.empty_title')}>
      {m('add_server.directory.empty_body')}
    </EmptyState>
  {:else}
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {#each entries as entry (entry.origin)}
        {@const profile = liveProfiles.get(entry.origin) ?? entry.profile}
        {@const joined = serverRegistry.findServerByUrl(entry.origin)}
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
          badge={joined ? m('add_server.directory.joined') : undefined}
          iconHref={external ? entry.origin : undefined}
          iconOpensInNewTab={external}
          onIconClick={external || (!joined && !canJoin(profile))
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
{/snippet}

<!-- The recommendations lead. The direct address lookup opens from a button,
     or shows at once when there is nothing to recommend. The page frames the
     directory in a titled Panel; the dialog already owns one work plane, so
     the directory sits directly on it. -->
{#if inDialog}
  <section class="flex flex-col gap-4" aria-labelledby="server-directory-recommended-title">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div class="flex min-w-0 flex-col gap-1">
        <h3
          id="server-directory-recommended-title"
          class="text-base font-semibold text-balance text-text-top"
        >
          {m('add_server.directory.servers_title')}
          {#if entries.length}
            <span class="font-normal text-muted tabular-nums">({entries.length})</span>
          {/if}
        </h3>
        <p class="text-sm text-pretty text-muted">
          {m('add_server.directory.servers_description')}
        </p>
      </div>
      {#if !showLookup}
        {@render lookupButton()}
      {/if}
    </div>
    {@render recommendationsBody()}
  </section>
{:else}
  <Panel
    title={m('add_server.directory.servers_title')}
    subtitle={m('add_server.directory.servers_description')}
    count={entries.length || undefined}
    actions={showLookup ? undefined : lookupButton}
  >
    <div class="flex flex-col gap-4">{@render recommendationsBody()}</div>
  </Panel>
{/if}
