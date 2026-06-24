<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import type {
    AdminEventLogEntry,
    AdminEventLogFilter
  } from '$lib/state/server/adminEventLog.svelte';
  import { Panel, DataTable } from '$lib/components/admin';
  import UserCombobox from '$lib/components/users/UserCombobox.svelte';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Combobox } from '$lib/ui/form';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDateTime as formatDateTimeUtil, formatDayLabel } from '$lib/utils/formatTime';
  import * as m from '$lib/i18n/messages';

  const userSettings = getUserSettings();

  const activeServerId = $derived(getActiveServer());
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const eventLog = $derived(stores.adminEventLog);

  let scrollContainer = $state<HTMLDivElement>();
  let loadedUrlKey = '';
  let draftEventType = $state('');
  let draftEventTypeText = $state('');
  let draftActorId = $state('');
  let draftActorText = $state('');

  const activeFilter = $derived(filterFromUrl(page.url));
  const activeFilterKey = $derived(filterKey(activeFilter));
  const draftFilter = $derived<AdminEventLogFilter>({
    eventType: draftEventType.trim(),
    actorId: draftActorId.trim(),
    createdAtFrom: '',
    createdAtTo: ''
  });
  const draftFilterKey = $derived(filterKey(draftFilter));
  const hasDraftChanges = $derived(draftFilterKey !== activeFilterKey);
  const formattedTotalCount = $derived(formatTotalCount(eventLog.totalCount));
  const loadedUrlAndServerKey = $derived(`${activeServerId}:${activeFilterKey}`);
  const eventTypeItems = $derived.by(() => {
    const query = draftEventTypeText.trim().toLowerCase();
    return eventLog.eventTypes
      .filter((eventType) => !query || eventType.toLowerCase().includes(query))
      .slice(0, 30)
      .map((eventType) => ({ value: eventType, label: eventType }));
  });

  $effect(() => {
    const key = loadedUrlAndServerKey;
    if (key === loadedUrlKey) return;

    loadedUrlKey = key;
    const filter = activeFilter;
    draftEventType = filter.eventType;
    draftEventTypeText = filter.eventType;
    draftActorId = filter.actorId;
    draftActorText = filter.actorId;
    void eventLog.loadEventTypes();
    void eventLog.loadFirstPage(filter);
  });

  function filterFromUrl(url: URL): AdminEventLogFilter {
    return {
      eventType: url.searchParams.get('eventType') ?? '',
      actorId: url.searchParams.get('actorId') ?? '',
      createdAtFrom: '',
      createdAtTo: ''
    };
  }

  function filterKey(filter: AdminEventLogFilter): string {
    return JSON.stringify(filter);
  }

  function formatTotalCount(count: string): string {
    const numeric = Number(count);
    return Number.isSafeInteger(numeric) ? numeric.toLocaleString() : count;
  }

  function formatTimestamp(iso: string): string {
    return formatDateTimeUtil(iso, userSettings);
  }

  function formatDateGroup(iso: string): string {
    return formatDayLabel(iso, userSettings);
  }

  function dateGroupKey(iso: string): string {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return 'unknown';
    const formatter = new Intl.DateTimeFormat('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      timeZone: userSettings.effectiveTimezone
    });
    return formatter.format(date);
  }

  function applyFilters() {
    if (!hasDraftChanges) return;
    navigateWithFilter(draftFilter);
  }

  function clearFilters() {
    draftEventType = '';
    draftEventTypeText = '';
    draftActorId = '';
    draftActorText = '';
    navigateWithFilter({
      eventType: '',
      actorId: '',
      createdAtFrom: '',
      createdAtTo: ''
    });
  }

  function loadOlderScanWindow() {
    void eventLog.loadMore();
  }

  function navigateWithFilter(filter: AdminEventLogFilter) {
    const params = new SvelteURLSearchParams();
    if (filter.eventType) params.set('eventType', filter.eventType);
    if (filter.actorId) params.set('actorId', filter.actorId);
    if (filter.createdAtFrom) params.set('from', filter.createdAtFrom);
    if (filter.createdAtTo) params.set('to', filter.createdAtTo);

    const query = params.toString();
    goto(
      resolve(
        query
          ? `/chat/[serverId]/server-admin/event-log?${query}`
          : '/chat/[serverId]/server-admin/event-log',
        {
          serverId: serverIdToSegment(activeServerId)
        }
      ),
      { keepFocus: true, noScroll: true }
    );
  }

  function openEntry(entry: AdminEventLogEntry) {
    goto(
      resolve('/chat/[serverId]/server-admin/event-log/[sequence]', {
        serverId: serverIdToSegment(activeServerId),
        sequence: entry.sequence
      })
    );
  }
</script>

<PageTitle title={m['admin.common.page_title']({ title: m['admin.event_log.title']() })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.event_log.title']()}
    subtitle={m['admin.event_log.subtitle']()}
    showMobileNav
  />

  <div class="min-h-0 flex-1 overflow-y-auto" bind:this={scrollContainer}>
    <div class="flex flex-col gap-4 p-6">
      {#if eventLog.error}
        <Hint tone="danger">{eventLog.error}</Hint>
      {/if}

      {#if eventLog.compatibilityMessage}
        <Hint tone="warning">{eventLog.compatibilityMessage}</Hint>
      {/if}

      {#if eventLog.scanLimited}
        <Hint tone="warning">
          <span class="flex flex-wrap items-center gap-3">
            <span>
              {m['admin.event_log.filtered_scan']({
                limit: eventLog.scanLimit.toLocaleString()
              })}
            </span>
            {#if eventLog.hasOlder}
              <Button
                variant="secondary"
                size="sm"
                onclick={loadOlderScanWindow}
                disabled={eventLog.loadingMore}
              >
                {m['admin.event_log.scan_older']()}
              </Button>
            {/if}
          </span>
        </Hint>
      {/if}

      <Panel title={m['admin.event_log.filters']()}>
        <div class="flex flex-col gap-4">
          <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <Combobox
              id="event-log-event-type"
              label={m['admin.event_log.event_type']()}
              bind:value={draftEventType}
              bind:text={draftEventTypeText}
              items={eventTypeItems}
              getValue={(item) => item.value}
              getLabel={(item) => item.label}
              placeholder={eventLog.eventTypesUnsupported
                ? m['admin.event_log.event_type_placeholder']()
                : m['admin.event_log.event_type_search_placeholder']()}
              loading={eventLog.eventTypesLoading}
              emptyMessage={m['admin.event_log.no_event_types']()}
              clearLabel={m['admin.event_log.clear_event_type']()}
            />

            <UserCombobox
              id="event-log-actor"
              label="Actor"
              bind:value={draftActorId}
              bind:text={draftActorText}
            />
          </div>

          <div class="flex flex-wrap justify-end gap-2">
            <Button
              variant="secondary"
              onclick={clearFilters}
              disabled={!eventLog.hasActiveFilter && !hasDraftChanges}
            >
              {m['admin.event_log.clear']()}
            </Button>
            <Button onclick={applyFilters} disabled={!hasDraftChanges || eventLog.loading}>
              {m['admin.event_log.apply']()}
            </Button>
          </div>
        </div>
      </Panel>

      <div class="text-sm text-muted">
        {eventLog.totalCount === '1'
          ? m['admin.event_log.total_events_one']({ count: formattedTotalCount })
          : m['admin.event_log.total_events_many']({ count: formattedTotalCount })}
        {#if eventLog.hasActiveFilter}
          · {eventLog.scannedCount === 1
            ? m['admin.event_log.inspected_rows_one']({
                count: eventLog.scannedCount.toLocaleString()
              })
            : m['admin.event_log.inspected_rows_many']({
                count: eventLog.scannedCount.toLocaleString()
              })}
        {/if}
      </div>

      <Panel noPadding>
        <DataTable
          items={eventLog.entries}
          columns={5}
          emptyMessage={eventLog.loading
            ? m['admin.common.loading']()
            : m['admin.event_log.no_matches']()}
          hasMore={eventLog.hasOlder && !eventLog.scanLimited && !eventLog.error}
          loadingMore={eventLog.loadingMore}
          onLoadMore={() => eventLog.loadMore()}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage={m['admin.event_log.loading_older']()}
          getGroupKey={(entry) => dateGroupKey(entry.createdAt)}
          onRowClick={openEntry}
        >
          {#snippet header()}
            <th class="px-4 py-3 font-medium">{m['admin.event_log.seq']()}</th>
            <th class="px-4 py-3 font-medium">{m['admin.event_log.time']()}</th>
            <th class="px-4 py-3 font-medium">{m['admin.event_log.event']()}</th>
            <th class="px-4 py-3 font-medium">{m['admin.event_log.aggregate']()}</th>
            <th class="px-4 py-3 font-medium">{m['admin.event_log.actor']()}</th>
          {/snippet}
          {#snippet row(entry)}
            <td class="px-4 py-3 font-mono text-sm text-muted">{entry.sequence}</td>
            <td class="px-4 py-3 text-sm">{formatTimestamp(entry.createdAt)}</td>
            <td class="px-4 py-3">
              <Pill tone="accent">{entry.eventType || '—'}</Pill>
            </td>
            <td class="px-4 py-3 font-mono text-xs">
              {#if entry.aggregateType}
                <span class="text-muted">{entry.aggregateType}.</span>{entry.aggregateId}
              {:else}
                <span class="text-muted">{entry.subject}</span>
              {/if}
            </td>
            <td class="px-4 py-3 font-mono text-xs">{entry.actorId || '—'}</td>
          {/snippet}
          {#snippet group(entry)}
            <div class="text-xs font-medium tracking-wide text-muted uppercase">
              {formatDateGroup(entry.createdAt)}
            </div>
          {/snippet}
        </DataTable>
      </Panel>
    </div>
  </div>
</div>
