<script lang="ts">
  import { ConnectError } from '@connectrpc/connect';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import {
    getPublicServerInfo,
    InvalidPublicServerError,
    type PublicServerInfo
  } from '$lib/api-client/server';
  import { startRemoteReauthentication, startServerOAuthFlow } from '$lib/auth/reauth';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import ServerTestimonialCard from '$lib/components/ServerTestimonialCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    canonicalServerOrigin,
    createServerDirectoryDiscovery,
    type ServerDirectoryDiscovery,
    type ServerDirectoryEntry,
    type ServerDirectorySnapshot
  } from '$lib/serverDirectory';
  import { evaluateServerCompatibility } from '$lib/state/server/compatibility';
  import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';
  import { EmptyState, Hint, PageTitle, PaneContent, PaneHeader, Panel } from '$lib/ui';
  import { Button, Form, TextInput } from '$lib/ui/form';

  let customInput = $state('');
  let customOrigin = $state('');
  let customProfile = $state<PublicServerInfo | null>(null);
  let customError = $state('');
  let probing = $state(false);
  let pendingOrigin = $state<string | null>(null);
  let actionError = $state('');
  let directoryState = $state<ServerDirectorySnapshot | null>(null);
  let directorySession: ServerDirectoryDiscovery | null = null;
  let unsubscribeDirectory: (() => void) | null = null;

  const registeredOrigins = $derived.by(() => [
    ...new Set(
      serverRegistry.servers.flatMap((server) => {
        const origin = canonicalServerOrigin(server.url);
        return origin ? [origin] : [];
      })
    )
  ]);
  const entries = $derived(directoryState?.entries.filter((entry) => entry.profile !== null) ?? []);
  const allSourcesFailed = $derived(
    !!directoryState &&
      !directoryState.isLoading &&
      directoryState.sourceCount > 0 &&
      directoryState.failedSourceCount === directoryState.sourceCount
  );
  const someSourcesFailed = $derived(
    !!directoryState && directoryState.failedSourceCount > 0 && !allSourcesFailed
  );

  onMount(() => {
    startDirectoryDiscovery();
    return stopDirectoryDiscovery;
  });

  function startDirectoryDiscovery() {
    stopDirectoryDiscovery();
    directoryState = null;
    const session = createServerDirectoryDiscovery(registeredOrigins, {
      initiallyVisible: document.visibilityState === 'visible'
    });
    directorySession = session;
    unsubscribeDirectory = session.subscribe((snapshot) => {
      if (directorySession === session) directoryState = snapshot;
    });
    session.start();
  }

  function stopDirectoryDiscovery() {
    unsubscribeDirectory?.();
    unsubscribeDirectory = null;
    directorySession?.cancel();
    directorySession = null;
  }

  function handleVisibilityChange() {
    directorySession?.setVisible(document.visibilityState === 'visible');
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

  function actionLabel(origin: string, profile: PublicServerInfo | null): string {
    const joined = registeredServer(origin);
    if (joined) {
      return serverRegistry.isAuthenticated(joined.id)
        ? m('add_server.directory.open')
        : m('add_server.sign_in');
    }
    if (opensInServerClient(origin, profile)) return m('add_server.directory.open_in_new_tab');
    if (!profile?.authorizeUrl) return m('add_server.directory.sign_in_unavailable');
    return m('add_server.directory.join');
  }

  function opensInServerClient(origin: string, profile: PublicServerInfo | null): boolean {
    return (
      !registeredServer(origin) &&
      profile !== null &&
      evaluateServerCompatibility({ serverVersion: profile.version }).status !== 'supported'
    );
  }

  async function openOrJoin(origin: string, profile: PublicServerInfo | null) {
    actionError = '';
    const joined = registeredServer(origin);
    if (!joined && (!profile || !profile.authorizeUrl)) return;
    pendingOrigin = origin;
    try {
      if (joined && serverRegistry.isAuthenticated(joined.id)) {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(joined.id) }));
      } else if (joined) {
        await startRemoteReauthentication(joined);
      } else if (profile) {
        await startServerOAuthFlow(origin, profile);
      }
    } catch {
      actionError = m('add_server.start_failed');
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

  function cardDisabled(entry: ServerDirectoryEntry): boolean {
    return (
      !registeredServer(entry.origin) &&
      !opensInServerClient(entry.origin, entry.profile) &&
      !entry.profile?.authorizeUrl
    );
  }

  function sourceName(origin: string): string {
    const registered = registeredServer(origin);
    if (registered) return registered.name;
    const discovered = entries.find((entry) => entry.origin === origin);
    if (discovered?.profile?.name) return discovered.profile.name;
    try {
      return new URL(origin).host;
    } catch {
      return origin;
    }
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

  function testimonialRecommendations(entry: ServerDirectoryEntry) {
    return entry.recommendations.flatMap((recommendation) =>
      recommendation.testimonial
        ? [
            {
              sourceOrigin: recommendation.sourceOrigin,
              sourceName: sourceName(recommendation.sourceOrigin),
              sourceIconUrl:
                registeredServer(recommendation.sourceOrigin)?.iconUrl ??
                entries.find((candidate) => candidate.origin === recommendation.sourceOrigin)?.profile
                  ?.iconUrl ??
                null,
              testimonial: recommendation.testimonial
            }
          ]
        : []
    );
  }
</script>

<svelte:document onvisibilitychange={handleVisibilityChange} />

<PageTitle title={m('add_server.directory.title')} />

<div class="pane-page">
  <PaneHeader
    title={m('add_server.directory.title')}
    subtitle={m('add_server.directory.subtitle')}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={m('add_server.directory.custom_title')}>
        <Form onsubmit={probeCustomServer} error={customError} maxWidth="max-w-2xl">
          <div class="flex flex-col items-stretch gap-3 sm:flex-row sm:items-end">
            <div class="min-w-0 flex-1">
              <TextInput
                id="add-server-url"
                label={m('add_server.url_label')}
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
          <div class="mt-5 max-w-md">
            {#snippet customActions()}
              {#if external}
                <Button
                  href={customOrigin}
                  opensInNewTab
                  variant="secondary"
                  fullWidth
                >
                  <span>{actionLabel(customOrigin, customProfile)}</span>
                  <span
                    class="iconify icon-[uil--external-link-alt]"
                    aria-hidden="true"
                  ></span>
                </Button>
              {:else}
                <Button
                  variant={joined ? 'secondary' : 'action'}
                  fullWidth
                  loading={pendingOrigin === customOrigin}
                  disabled={!joined && !profile.authorizeUrl}
                  onclick={() => openOrJoin(customOrigin, profile)}
                >
                  {actionLabel(customOrigin, customProfile)}
                </Button>
              {/if}
            {/snippet}
            <ServerProfileCard
              origin={customOrigin}
              {profile}
              badge={joined ? m('add_server.directory.joined') : undefined}
              iconHref={external ? customOrigin : undefined}
              iconOpensInNewTab={external}
              onIconClick={external || (!joined && !profile.authorizeUrl)
                ? undefined
                : () => openOrJoin(customOrigin, profile)}
              iconActionLabel={actionLabel(customOrigin, profile)}
              iconActionDisabled={pendingOrigin === customOrigin}
              actions={customActions}
              testId="server-directory-entry"
            />
          </div>
        {/if}
      </Panel>

      <Panel
        title={m('add_server.directory.servers_title')}
        subtitle={m('add_server.directory.servers_description')}
        count={entries.length || undefined}
      >
        {#if actionError}
          <div class="mb-4"><Hint tone="danger">{actionError}</Hint></div>
        {/if}
        {#if someSourcesFailed}
          <div class="mb-4"><Hint tone="warning">{m('add_server.directory.partial')}</Hint></div>
        {/if}

        {#if !directoryState || directoryState.isInitialLoading}
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
            {#each Array(3) as _, index (index)}
              <div class="skeleton h-64 rounded-xl bg-surface"></div>
            {/each}
          </div>
        {:else if allSourcesFailed}
          <EmptyState
            icon="icon-[uil--exclamation-triangle]"
            title={m('add_server.directory.unavailable_title')}
          >
            <div class="flex flex-col items-center gap-3">
              <span>{m('add_server.directory.unavailable_body')}</span>
              <Button variant="secondary" onclick={startDirectoryDiscovery}>
                {m('common.retry')}
              </Button>
            </div>
          </EmptyState>
        {:else if entries.length === 0}
          <EmptyState icon="icon-[uil--compass]" title={m('add_server.directory.empty_title')}>
            {m('add_server.directory.empty_body')}
          </EmptyState>
        {:else}
          <div class="columns-1 gap-4 sm:columns-2 lg:columns-3">
            {#each entries as entry (entry.origin)}
              {@const joined = registeredServer(entry.origin)}
              {@const external = opensInServerClient(entry.origin, entry.profile)}
              {@const attribution = sourceAttribution(entry)}
              {@const testimonials = testimonialRecommendations(entry)}
              {#snippet cardActions()}
                <div class="flex flex-col gap-3">
                  <p
                    class="line-clamp-2 text-sm text-muted"
                    aria-label={attribution.full}
                    title={attribution.full}
                    data-testid="server-recommendation-sources"
                  >
                    <bdi>{attribution.visible}</bdi>
                  </p>
                  {#if external}
                    <Button
                      href={entry.origin}
                      opensInNewTab
                      variant="secondary"
                      fullWidth
                    >
                      <span>{actionLabel(entry.origin, entry.profile)}</span>
                      <span
                        class="iconify icon-[uil--external-link-alt]"
                        aria-hidden="true"
                      ></span>
                    </Button>
                  {:else}
                    <Button
                      variant={joined ? 'secondary' : 'action'}
                      fullWidth
                      loading={pendingOrigin === entry.origin}
                      disabled={cardDisabled(entry)}
                      onclick={() => openOrJoin(entry.origin, entry.profile)}
                    >
                      {actionLabel(entry.origin, entry.profile)}
                    </Button>
                  {/if}
                </div>
              {/snippet}
              <div class="mb-4 break-inside-avoid">
                <ServerProfileCard
                  origin={entry.origin}
                  profile={entry.profile}
                  badge={joined ? m('add_server.directory.joined') : undefined}
                  iconHref={external ? entry.origin : undefined}
                  iconOpensInNewTab={external}
                  onIconClick={external || cardDisabled(entry)
                    ? undefined
                    : () => openOrJoin(entry.origin, entry.profile)}
                  iconActionLabel={actionLabel(entry.origin, entry.profile)}
                  iconActionDisabled={pendingOrigin === entry.origin}
                  actions={cardActions}
                  testId="server-directory-entry"
                />
                {#if testimonials.length > 0}
                  <section
                    class="mt-2 flex flex-col gap-2"
                    aria-label={m('add_server.directory.testimonials_for', {
                      server: entry.profile?.name ?? entry.origin
                    })}
                    data-testid="server-testimonials"
                  >
                    {#each testimonials as testimonial (testimonial.sourceOrigin)}
                      <ServerTestimonialCard
                        testimonial={testimonial.testimonial}
                        sourceName={testimonial.sourceName}
                        sourceIconUrl={testimonial.sourceIconUrl}
                      />
                    {/each}
                  </section>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
        {#if directoryState && !allSourcesFailed}
          {#if directoryState.isLoading && !directoryState.isInitialLoading}
            <p class="mt-4 text-center text-muted" aria-live="polite">
              {m('add_server.directory.discovering')}
            </p>
          {/if}
          {#if directoryState.sessionLimitReached}
            <div class="mt-4">
              <Hint tone="warning">{m('add_server.directory.session_limit_reached')}</Hint>
            </div>
          {:else if directoryState.canLoadMore}
            <div class="mt-4 flex justify-center">
              <Button variant="secondary" onclick={() => directorySession?.loadMore()}>
                {m('add_server.directory.load_more')}
              </Button>
            </div>
          {/if}
        {/if}
      </Panel>
    </div>
  </PaneContent>
</div>
