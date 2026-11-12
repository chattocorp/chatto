<!-- @component Bundled release highlights and advisory features from the selected server. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { version as frontendVersion } from '../../../../package.json';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { createQuery } from '$lib/query/client';
  import { serverQueryRoot, serverSessionQueryRoot } from '$lib/query/keys';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { hasServerScope, useServerScope } from '$lib/state/server/scope.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { PageTitle, PaneHeader, PaneContent, Panel, Hint, LoadingFog, UnreadDot } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import HelpDocument from '../HelpDocument.svelte';
  import {
    createAccountAPI,
    mergeReleaseHighlightState,
    type AccountUserSettings
  } from '@chatto/client/api/account';
  import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
  import { listServerFeatures } from '@chatto/client/api/server';
  import {
    limitedFrontendFeatures,
    normalizeServerVersion
  } from '@chatto/client/server/compatibility';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { applicableHighlights, unreadHighlights, type HighlightSurface } from './catalog';

  import { loadHighlight } from './articles';
  import { withHelpServer } from '../links';

  let { surface = 'users' }: { surface?: HighlightSurface } = $props();
  const scope = hasServerScope() ? useServerScope() : undefined;
  // The shared scope provider owns the server/account lifetime. Public history
  // can also render without an account and cannot mutate private state.
  const guard = scope ? createSessionGuard(scope, 'server-session') : undefined;
  const store = scope?.store;
  const connection = scope?.connection;
  const entries = $derived(
    applicableHighlights(surface, frontendVersion, store?.serverInfo.version)
  );
  const highlightState = $derived(store?.currentUser.user?.settings?.releaseHighlights);
  const authenticated = $derived(!!store?.currentUser.verifiedUserId);
  const unread = $derived(authenticated ? unreadHighlights(surface, entries, highlightState) : []);
  const baseline = $derived(
    surface === 'users' ? highlightState?.usersSinceVersion : highlightState?.adminsSinceVersion
  );
  const supportsDismissal = $derived(authenticated && highlightState !== undefined);
  const canDismiss = $derived(supportsDismissal && baseline !== undefined);
  let saving = $state(false);
  let saveError = $state<unknown>(null);

  /** Merge only this private monotonic state, without replacing other preferences. */
  function applySettings(settings: AccountUserSettings, userId: string) {
    if (!settings.releaseHighlights || !store) return;
    const incoming = settings.releaseHighlights;
    store.currentUser.update(userId, (user) => ({
      settings: {
        ...user.settings,
        timeFormat: user.settings?.timeFormat ?? TimeFormat.TIME_FORMAT_AUTO,
        releaseHighlights: mergeReleaseHighlightState(user.settings?.releaseHighlights, incoming)
      }
    }));
  }

  const initialState = createQuery(() => ({
    queryKey: [
      ...(scope && connection
        ? serverSessionQueryRoot(scope.serverId, connection)
        : ['release-highlights']),
      'release-highlights-baseline',
      surface,
      entries[0]?.version
    ],
    enabled: supportsDismissal && baseline === undefined && entries.length > 0,
    retry: false,
    queryFn: async ({ signal }) => {
      if (!connection || !store || !guard || !entries[0]) return null;
      const snapshot = guard.snapshot();
      const userId = store.currentUser.verifiedUserId;
      if (!userId) return null;
      const result = await connection
        .getAPI(createAccountAPI)
        .initializeReleaseHighlights(surface, entries[0].version, { signal });
      if (guard.isCurrent(snapshot) && store.currentUser.verifiedUserId === userId)
        applySettings(result, userId);
      return result;
    }
  }));

  async function dismiss(ids: string[]) {
    if (!connection || !store || !guard || !canDismiss || saving) return;
    const userId = store.currentUser.verifiedUserId;
    if (!userId) return;
    const snapshot = guard.snapshot();
    saving = true;
    saveError = null;
    try {
      // Bounded calls acknowledge only the visible IDs, never a future release.
      for (let offset = 0; offset < ids.length; offset += 100) {
        const result = await connection
          .getAPI(createAccountAPI)
          .dismissReleaseHighlights(ids.slice(offset, offset + 100));
        if (!guard.isCurrent(snapshot) || store.currentUser.verifiedUserId !== userId) return;
        applySettings(result, userId);
      }
    } catch (error) {
      if (guard.isCurrent(snapshot)) saveError = error;
    } finally {
      // Pending state belongs to this component, even when a privacy refresh
      // fences the response. Clearing it cannot apply retired account data.
      saving = false;
    }
  }

  const catalogEnabled = $derived(
    !!scope &&
      !!connection &&
      !store?.serverInfo.isOriginServer &&
      store?.serverInfo.lastDiscoveredAt !== null &&
      store?.serverInfo.error === null
  );
  const catalog = createQuery(() => {
    const locale = getReactiveLocale();
    const frontendRelease = normalizeServerVersion(frontendVersion) ?? undefined;
    return {
      queryKey: [
        ...serverQueryRoot(scope?.serverId ?? ''),
        'public-features',
        store?.serverInfo.version,
        locale,
        frontendRelease
      ],
      enabled: catalogEnabled,
      queryFn: ({ signal }) =>
        listServerFeatures(connection!.connectBaseUrl, {
          locale,
          introducedAfterVersion: frontendRelease,
          signal
        }),
      staleTime: Infinity
    };
  });
  const missing = $derived(
    catalogEnabled ? limitedFrontendFeatures(catalog.data ?? [], frontendVersion) : []
  );
  const unreadIds = $derived(new Set(unread.map((entry) => entry.id)));

  /** Anchor navigation waits for its lazily compiled article to mount. */
  function restoreAnchor(element: HTMLElement) {
    if (window.location.hash === `#highlight-${element.dataset.highlight}`)
      element.closest('section')?.scrollIntoView({ block: 'start' });
  }
</script>

<PageTitle title={m('ui.release_highlights.title')} />
<div class="pane-page">
  <PaneHeader
    title={m('ui.release_highlights.title')}
    subtitle={store?.serverInfo.name}
    backHref={surface === 'users' && sidebarNav.isMobile
      ? resolve(
          withHelpServer('/help', scope ? serverIdToSegment(scope.serverId) : undefined) as '/'
        )
      : undefined}
    backLabel={m('ui.help')}
  />
  <PaneContent wide>
    <div class="mx-auto flex max-w-3xl flex-col gap-6">
      {#if authenticated && unread.length === 0}<Hint>{m('ui.release_highlights.empty')}</Hint>{/if}
      {#if canDismiss && unread.length > 0}
        <div>
          <Button
            variant="secondary"
            disabled={saving || initialState.isFetching}
            onclick={() => void dismiss(unread.map((entry) => `${surface}:${entry.id}`))}
            >{m('ui.release_highlights.dismiss_all')}</Button
          >
        </div>
      {:else if authenticated && !supportsDismissal}
        <Hint>{m('ui.release_highlights.unsupported')}</Hint>
      {/if}
      {#if saveError || initialState.error}
        <Hint tone="danger"
          >{errorMessage(saveError ?? initialState.error, m('common.error.generic'))}
          {#if initialState.error}<Button
              variant="secondary"
              onclick={() => void initialState.refetch()}>{m('common.retry')}</Button
            >{/if}
        </Hint>
      {/if}
      {#each entries as entry (entry.id)}
        <section id={`highlight-${entry.id}`} class="scroll-mt-4" aria-label={m(entry.titleKey)}>
          <Panel title={m(entry.titleKey)}>
            <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <span class="text-muted" dir="ltr">{entry.version}</span>
              {#if unreadIds.has(entry.id)}
                <span class="flex items-center gap-2"
                  ><UnreadDot color="action" />{m('ui.release_highlights.new')}</span
                >
              {/if}
              {#if canDismiss && unreadIds.has(entry.id)}
                <Button
                  variant="ghost"
                  disabled={saving || initialState.isFetching}
                  onclick={() => void dismiss([`${surface}:${entry.id}`])}
                  >{m('ui.release_highlights.dismiss')}</Button
                >
              {/if}
            </div>
            {#await loadHighlight(getReactiveLocale(), surface, entry.id)}
              <LoadingFog class="h-24" />
            {:then { default: Article, locale }}
              <div data-highlight={entry.id} {@attach restoreAnchor}>
                <HelpDocument {locale}><Article /></HelpDocument>
              </div>
            {:catch}
              <Hint tone="danger">{m('common.error.generic')}</Hint>
            {/await}
          </Panel>
        </section>
      {/each}
      {#if surface === 'users' && catalogEnabled && (catalog.isPending || catalog.error || missing.length > 0)}
        <Panel title={m('ui.release_highlights.newer_app')}>
          {#if catalog.isPending}<LoadingFog class="h-24" />
          {:else if catalog.error}
            <Hint tone="warning">{errorMessage(catalog.error, m('common.error.generic'))}</Hint>
            <Button variant="secondary" onclick={() => void catalog.refetch()}
              >{m('common.retry')}</Button
            >
          {:else}
            <p class="mb-4">
              {m('chat.compatibility.frontend_upgrade', { version: frontendVersion })}
            </p>
            <ul class="flex flex-col gap-4">
              {#each missing as feature (feature.id)}
                <li>
                  <h3 class="font-semibold">{feature.name}</h3>
                  <p>{feature.description}</p>
                  <p class="text-muted" dir="ltr">{feature.introducedVersion}</p>
                </li>
              {/each}
            </ul>
            {#if scope}<p class="mt-4">
                <a
                  class="link"
                  href={resolve(
                    `/chat/${serverIdToSegment(scope.serverId)}/info#compatibility` as '/'
                  )}>{m('chat.compatibility.title')}</a
                >
              </p>{/if}
          {/if}
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
