<script lang="ts">
  import { ConnectError } from '@connectrpc/connect';
  import { createQuery } from '@tanstack/svelte-query';
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
  import { queryClient } from '$lib/query/client';
  import {
    canonicalServerOrigin,
    loadServerDirectory,
    type ServerDirectoryEntry
  } from '$lib/serverDirectory';
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

  const registeredOrigins = $derived.by(() => [
    ...new Set(
      serverRegistry.servers.flatMap((server) => {
        const origin = canonicalServerOrigin(server.url);
        return origin ? [origin] : [];
      })
    )
  ]);
  const directoryQuery = createQuery(
    () => ({
      queryKey: ['public', 'server-directory', registeredOrigins],
      queryFn: ({ signal }) => loadServerDirectory(registeredOrigins, { signal }),
      staleTime: 0,
      refetchOnMount: 'always'
    }),
    () => queryClient
  );
  const entries = $derived(
    directoryQuery.data?.entries.filter((entry) => entry.profile !== null) ?? []
  );
  const allSourcesFailed = $derived(
    !!directoryQuery.data &&
      directoryQuery.data.sourceCount > 0 &&
      directoryQuery.data.failedSourceCount === directoryQuery.data.sourceCount
  );
  const someSourcesFailed = $derived(
    !!directoryQuery.data && directoryQuery.data.failedSourceCount > 0 && !allSourcesFailed
  );

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
    if (!profile?.authorizeUrl) return m('add_server.directory.sign_in_unavailable');
    return m('add_server.directory.join');
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
    return !registeredServer(entry.origin) && !entry.profile?.authorizeUrl;
  }

  function sourceName(origin: string): string {
    const registered = registeredServer(origin);
    if (registered) return registered.name;
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
              sourceIconUrl: registeredServer(recommendation.sourceOrigin)?.iconUrl ?? null,
              testimonial: recommendation.testimonial
            }
          ]
        : []
    );
  }
</script>

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
          <div class="mt-5 max-w-md">
            {#snippet customActions()}
              <Button
                variant={joined ? 'secondary' : 'action'}
                fullWidth
                loading={pendingOrigin === customOrigin}
                disabled={!joined && !profile.authorizeUrl}
                onclick={() => openOrJoin(customOrigin, profile)}
              >
                {actionLabel(customOrigin, customProfile)}
              </Button>
            {/snippet}
            <ServerProfileCard
              origin={customOrigin}
              {profile}
              badge={joined ? m('add_server.directory.joined') : undefined}
              onIconClick={!joined && !profile.authorizeUrl
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

        {#if directoryQuery.isPending}
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
            {#each Array(3) as _, index (index)}
              <div class="skeleton h-64 rounded-xl bg-surface"></div>
            {/each}
          </div>
        {:else if directoryQuery.isError || allSourcesFailed}
          <EmptyState
            icon="icon-[uil--exclamation-triangle]"
            title={m('add_server.directory.unavailable_title')}
          >
            <div class="flex flex-col items-center gap-3">
              <span>{m('add_server.directory.unavailable_body')}</span>
              <Button variant="secondary" onclick={() => directoryQuery.refetch()}>
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
                  <Button
                    variant={joined ? 'secondary' : 'action'}
                    fullWidth
                    loading={pendingOrigin === entry.origin}
                    disabled={cardDisabled(entry)}
                    onclick={() => openOrJoin(entry.origin, entry.profile)}
                  >
                    {actionLabel(entry.origin, entry.profile)}
                  </Button>
                </div>
              {/snippet}
              <div class="mb-4 break-inside-avoid">
                <ServerProfileCard
                  origin={entry.origin}
                  profile={entry.profile}
                  badge={joined ? m('add_server.directory.joined') : undefined}
                  onIconClick={cardDisabled(entry)
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
      </Panel>
    </div>
  </PaneContent>
</div>
