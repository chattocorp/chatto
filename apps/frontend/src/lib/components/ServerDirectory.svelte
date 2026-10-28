<!--
@component

The Server Directory: registered servers, explicitly discovered servers,
their cached Neighborhoods, and a direct server-address lookup. This is the
single directory surface for every host and the standalone start page. See FDR-042.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { ConnectError } from '@connectrpc/connect';
  import { onMount } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { goto, pushState, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import {
    getPublicServerInfo,
    InvalidPublicServerError,
    type NeighborhoodServerProfile,
    type PublicServerInfo
  } from '@chatto/client/api/server';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    loadServerDirectory,
    type ServerDirectory,
    type ServerDirectoryEntry
  } from '$lib/serverDirectory';
  import { canonicalServerOrigin, serverHost } from '$lib/serverUrl';
  import { evaluateServerCompatibility } from '@chatto/client/server/compatibility';
  import { knownServers, serverRegistry } from '$lib/client';
  import { addSignedOutServer, findServerByUrl } from '$lib/serverCatalogue';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { Dialog, EmptyState, Hint, LoadingFog } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  /** A live profile from a direct lookup, or a cached Server Directory profile. */
  type ServerVersionProfile = PublicServerInfo | NeighborhoodServerProfile | null;

  /** The current profile does not let this client sign in. */
  class ServerJoinUnavailableError extends Error {}

  let {
    lookupRequested = false,
    onLookupClose
  }: {
    /** The pane header or lookup flow requested the direct address form. */
    lookupRequested?: boolean;
    /** Close the route-owned address dialog through browser history. */
    onLookupClose?: () => void;
  } = $props();
  let customInput = $state('');
  let customError = $state('');
  let probing = $state(false);
  let pendingOrigin = $state<string | null>(null);
  let directory = $state<ServerDirectory | null>(null);
  let directoryController: AbortController | null = null;
  let lookupController: AbortController | null = null;
  /** Current profiles that a join action loaded; they replace cached profiles. */
  const liveProfiles = new SvelteMap<string, PublicServerInfo>();

  const discoveryOrigins = $derived.by(() => [
    ...new Set(
      [
        ...serverRegistry.servers.map((server) => server.url),
        ...knownServers.servers.map((server) => server.origin)
      ].flatMap((url) => {
        const origin = canonicalServerOrigin(url);
        return origin ? [origin] : [];
      })
    )
  ]);
  const savedServers = $derived(serverRegistry.servers);
  const firstRun = $derived(savedServers.length === 0 && knownServers.servers.length === 0);
  const entries = $derived.by(() => {
    // Insertion order keeps explicitly added servers before recommendations,
    // even when a later recommendation also names a known server.
    const merged = new SvelteMap<string, ServerDirectoryEntry>();
    for (const server of knownServers.servers) {
      merged.set(server.origin, { ...server, imageOrigin: server.origin, sourceOrigins: [] });
    }
    for (const entry of directory?.entries ?? []) {
      const known = merged.get(entry.origin);
      merged.set(entry.origin, known ? { ...known, sourceOrigins: entry.sourceOrigins } : entry);
    }
    return [...merged.values()].filter((entry) => !findServerByUrl(entry.origin));
  });
  const allSourcesFailed = $derived(
    !!directory &&
      directory.sourceCount > 0 &&
      directory.failedSourceCount === directory.sourceCount
  );
  const someSourcesFailed = $derived(
    !!directory && directory.failedSourceCount > 0 && !allSourcesFailed
  );

  onMount(() => {
    void refreshDirectory();
    return () => directoryController?.abort();
  });

  /** Query registered and explicitly discovered sources, never their returned neighbors. */
  async function refreshDirectory() {
    directoryController?.abort();
    const controller = new AbortController();
    directoryController = controller;
    directory = null;
    try {
      const loaded = await loadServerDirectory(discoveryOrigins, { signal: controller.signal });
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
    if (probing) return;
    customError = '';
    const initialOrigin = canonicalServerOrigin(normalizeCustomInput(customInput));
    if (!initialOrigin) {
      customError = m('add_server.invalid_url');
      return;
    }

    probing = true;
    const controller = new AbortController();
    lookupController = controller;
    const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]);
    try {
      let origin = initialOrigin;
      let profile: PublicServerInfo;
      try {
        profile = await getPublicServerInfo(origin, { signal });
      } catch (error) {
        signal.throwIfAborted();
        if (hasScheme(customInput) || !origin.startsWith('https://')) throw error;
        origin = `http://${origin.slice('https://'.length)}`;
        profile = await getPublicServerInfo(origin, { signal });
      }
      signal.throwIfAborted();
      const wasFirstRun = firstRun;
      liveProfiles.set(origin, profile);
      knownServers.remember(origin, profile);
      customInput = '';
      if (lookupRequested && canJoin(profile) && !opensInServerClient(origin, profile)) {
        await openOrJoin(origin, profile);
        return;
      }
      onLookupClose?.();
      // First-run success remounts the directory, which loads these sources.
      // An existing directory refreshes here without registering the server.
      if (!wasFirstRun) void refreshDirectory();
    } catch (error) {
      if (!controller.signal.aborted) customError = discoveryError(error);
    } finally {
      if (lookupController === controller) {
        lookupController = null;
        probing = false;
      }
    }
  }

  /** Closing or replacing the address form cancels its pending lookup. */
  function lookupLifetime() {
    return () => {
      lookupController?.abort();
      lookupController = null;
      probing = false;
    };
  }

  function actionLabel(origin: string, profile: ServerVersionProfile): string {
    if (findServerByUrl(origin)) return m('add_server.directory.open');
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
      !findServerByUrl(origin) &&
      profile !== null &&
      evaluateServerCompatibility({ serverVersion: profile.version }).status === 'unsupported'
    );
  }

  /**
   * Open a registered server, or add a new one to the gutter and open it.
   * Joining does not submit credentials or open an authorization window.
   * The server's signed-out view offers sign-in. A Server Directory result
   * has only the cached profile, so adding it first loads the server's current
   * profile. The user starts this request explicitly.
   */
  async function openOrJoin(origin: string, profile: ServerVersionProfile) {
    // Clear the shallow dialog state before a full navigation. Replacing a
    // shallow entry with goto would make Back change only the URL, not the page.
    if (lookupRequested) replaceState('', {});
    const joined = findServerByUrl(origin);
    if (!joined && !canJoin(profile)) return;
    pendingOrigin = origin;
    try {
      if (joined) {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(joined.id) }), {
          replaceState: false
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
      const serverId = addSignedOutServer(origin, {
        name: current.name,
        iconUrl: current.iconUrl ?? null
      });
      await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }), {
        replaceState: false
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
    const known = knownServers.servers.find((server) => server.origin === origin);
    if (known) return known.profile.name || serverHost(origin);
    const registered = findServerByUrl(origin);
    if (registered) return registered.name;
    const discovered = entries.find((entry) => entry.origin === origin);
    if (discovered?.profile?.name) return discovered.profile.name;
    return serverHost(origin);
  }

  /** Keep the full cached provenance in the shallow history entry for Back/Close. */
  function showRecommendationInfo(entry: ServerDirectoryEntry) {
    pushState('', {
      modal: {
        type: 'serverRecommendation',
        name: entry.profile.name || serverHost(entry.origin),
        origin: entry.origin,
        sources: entry.sourceOrigins.map((origin) => ({ origin, name: sourceName(origin) }))
      }
    });
  }
</script>

{#snippet lookupForm(inDialog = false)}
  <section
    {@attach lookupLifetime}
    aria-label={inDialog ? m('add_server.directory.connect_by_address') : undefined}
    aria-labelledby={inDialog ? undefined : 'server-directory-connect-title'}
  >
    {#if !inDialog}
      <h2 id="server-directory-connect-title" class="mb-4 text-xl font-semibold text-text-top">
        {m('add_server.directory.connect_by_address')}
      </h2>
    {/if}
    <div class="flex flex-col gap-4" data-testid="server-directory-lookup">
      <p class="text-pretty text-text">{m('add_server.directory.lookup_description')}</p>
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
            <span class="iconify icon-[uil--search]" aria-hidden="true"></span>
            {m('add_server.directory.find')}
          </Button>
        </div>
      </Form>
    </div>
  </section>
{/snippet}

{#snippet directoryStatus()}
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
  {/if}
{/snippet}

{#snippet recommendedCards()}
  {#each entries as entry (entry.origin)}
    {@const profile = liveProfiles.get(entry.origin) ?? entry.profile}
    {@const joined = findServerByUrl(entry.origin)}
    {@const addedByYou = knownServers.servers.some((server) => server.origin === entry.origin)}
    {@const external = opensInServerClient(entry.origin, profile)}
    {#snippet provenance()}
      <div class="flex min-w-0 items-center justify-center text-muted">
        {#if pendingOrigin === entry.origin}
          <span role="status">{m('add_server.connecting')}</span>
        {:else if !canJoin(profile)}
          <span>{m('add_server.directory.sign_in_unavailable')}</span>
        {:else if addedByYou}
          <span>{m('add_server.directory.added_by_you')}</span>
        {:else if entry.sourceOrigins.length > 0}
          <button
            type="button"
            class="pointer-events-auto flex min-w-0 cursor-pointer items-center justify-center gap-1 rounded-sm text-center hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
            onclick={() => showRecommendationInfo(entry)}
            data-testid="server-recommendation-source"
          >
            <span class="truncate"
              ><bdi
                >{m('add_server.directory.recommended_by', {
                  servers: sourceName(entry.sourceOrigins[0]!)
                })}</bdi
              ></span
            >
            {#if entry.sourceOrigins.length > 1}
              <span class="shrink-0" dir="ltr">+{entry.sourceOrigins.length - 1}</span>
            {/if}
          </button>
        {/if}
      </div>
    {/snippet}
    <ServerProfileCard
      origin={entry.origin}
      imageOrigin={liveProfiles.has(entry.origin) ? entry.origin : entry.imageOrigin}
      {profile}
      href={external ? entry.origin : undefined}
      opensInNewTab={external}
      onactivate={external ? undefined : () => openOrJoin(entry.origin, profile)}
      actionLabel={actionLabel(entry.origin, profile)}
      disabled={pendingOrigin === entry.origin || (!joined && !canJoin(profile))}
      footer={provenance}
      testId="server-directory-entry"
      headingTag="h2"
    />
  {/each}
{/snippet}

<div
  class={['flex w-full flex-col gap-8', firstRun && 'mx-auto max-w-3xl text-center']}
  data-testid="server-directory"
>
  {#if firstRun}
    <!-- First-run onboarding deliberately uses an open layout rather than
         the pane panels used for settings and administration. -->
    <section class="flex flex-col items-center" aria-labelledby="server-directory-welcome-title">
      <div class="flex flex-col items-center gap-6">
        <h1
          id="server-directory-welcome-title"
          class="text-5xl leading-tight font-black tracking-tight text-balance text-text-top sm:text-6xl"
        >
          {m('add_server.directory.welcome_title')}
        </h1>
        <p class="max-w-2xl text-lg leading-relaxed text-balance text-text">
          {m('auth.login.welcome_description')}
        </p>
      </div>
    </section>
    <div class="mx-auto w-full max-w-xl">
      {@render lookupForm()}
    </div>
    <section
      class="mx-auto flex max-w-2xl flex-col items-center gap-4"
      aria-labelledby="server-directory-learn-more-title"
    >
      <h2 id="server-directory-learn-more-title" class="text-xl font-semibold text-text-top">
        {m('add_server.directory.learn_more')}
      </h2>
      <p class="text-balance text-text">
        {m('add_server.directory.learn_more_description')}
        <a href="https://chatto.run" target="_blank" rel="noopener noreferrer" class="link"
          >www.chatto.run</a
        >
      </p>
    </section>
  {/if}

  {#if !firstRun}
    <div
      class="grid grid-cols-[repeat(auto-fill,min(100%,18rem))] gap-4"
      data-testid="server-directory-grid"
    >
      {#each savedServers as server (server.id)}
        {@const info = serverRegistry.tryGetStore(server.id)?.serverInfo}
        {@const profile = {
          name: info ? serverDisplayName(info, server.name) : server.name,
          iconUrl: info?.iconUrl ?? server.iconUrl,
          bannerUrl: info?.bannerUrl ?? null,
          description: info?.description ?? null
        }}
        {@const href = resolve('/chat/[serverId]', { serverId: serverIdToSegment(server.id) })}
        {#snippet status()}
          <span
            >{serverRegistry.isAuthenticated(server.id)
              ? m('add_server.directory.joined')
              : m('add_server.directory.needs_login')}</span
          >
        {/snippet}
        <ServerProfileCard
          origin={server.url}
          {profile}
          {href}
          actionLabel={m('add_server.directory.open')}
          footer={status}
          testId="saved-server-entry"
          headingTag="h2"
        />
      {/each}
      {@render recommendedCards()}
    </div>
    {@render directoryStatus()}
  {/if}
</div>

{#if !firstRun && lookupRequested}
  <Dialog visible title={m('add_server.directory.connect_by_address')} onclose={onLookupClose}>
    {@render lookupForm(true)}
  </Dialog>
{/if}
