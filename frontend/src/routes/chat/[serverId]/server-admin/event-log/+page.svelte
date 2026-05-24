<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDateTime as formatDateTimeUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();

  const EventLogQuery = graphql(`
    query AdminEventLog($limit: Int, $before: String) {
      admin {
        eventLog(limit: $limit, before: $before) {
          entries {
            sequence
            subject
            aggregateType
            aggregateId
            eventType
            eventId
            actorId
            createdAt
          }
          hasOlder
          endCursor
          totalCount
        }
      }
    }
  `);

  // The page accumulates entries across pagination clicks; the query
  // itself only ever fetches one page at a time.
  const pageSize = 50;
  let beforeCursor = $state<string | null>(null);
  let accumulated = $state<Entry[]>([]);
  let totalCount = $state(0);
  let hasOlder = $state(false);
  let lastLoadedCursor = $state<string | null>(null);

  type Entry = {
    sequence: string;
    subject: string;
    aggregateType: string;
    aggregateId: string;
    eventType: string;
    eventId: string;
    actorId: string;
    createdAt: string;
  };

  const eventsQuery = useQuery(EventLogQuery, () => ({
    limit: pageSize,
    before: beforeCursor
  }));

  // When a fresh page lands, append it to the accumulator. Guard against
  // double-append by tracking the endCursor we last consumed.
  $effect(() => {
    const conn = eventsQuery.data?.admin?.eventLog;
    if (!conn) return;
    const cursorKey = conn.endCursor ?? '__head__';
    if (cursorKey === lastLoadedCursor) return;
    lastLoadedCursor = cursorKey;
    if (beforeCursor === null) {
      accumulated = [...conn.entries];
    } else {
      accumulated = [...accumulated, ...conn.entries];
    }
    totalCount = conn.totalCount;
    hasOlder = conn.hasOlder;
  });

  let loading = $derived(eventsQuery.loading);
  let error = $derived(
    eventsQuery.error ??
      (!eventsQuery.loading && !eventsQuery.data?.admin
        ? 'Event log unavailable (admin access required)'
        : null)
  );

  function formatTimestamp(iso: string): string {
    return formatDateTimeUtil(iso, userSettings);
  }

  function loadMore() {
    if (!hasOlder) return;
    const last = accumulated[accumulated.length - 1];
    if (!last) return;
    beforeCursor = last.sequence;
  }

  function openEntry(entry: Entry) {
    goto(
      resolve('/chat/[serverId]/server-admin/event-log/[sequence]', {
        serverId: serverIdToSegment(getActiveServer()),
        sequence: entry.sequence
      })
    );
  }
</script>

<PageTitle title="Event Log | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Event Log"
    subtitle="Browse the durable event-sourcing stream (EVT). Read-only — useful for debugging and as a starting point for the audit log."
    showMobileNav
  />

  <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-6">
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {/if}

    <div class="text-sm text-muted">
      {totalCount.toLocaleString()} total event{totalCount === 1 ? '' : 's'} in stream
    </div>

    <Panel noPadding>
      <DataTable
        items={accumulated}
        columns={5}
        emptyMessage={loading ? 'Loading…' : 'No events recorded yet.'}
        onRowClick={openEntry}
      >
        {#snippet header()}
          <th class="px-4 py-3 font-medium">Seq</th>
          <th class="px-4 py-3 font-medium">Time</th>
          <th class="px-4 py-3 font-medium">Event</th>
          <th class="px-4 py-3 font-medium">Aggregate</th>
          <th class="px-4 py-3 font-medium">Actor</th>
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
      </DataTable>
    </Panel>

    {#if hasOlder}
      <div class="flex justify-center">
        <button
          type="button"
          class="cursor-pointer rounded-lg border border-text/10 bg-surface-100 px-4 py-2 text-sm hover:bg-surface-200"
          onclick={loadMore}
          disabled={loading}
        >
          {loading ? 'Loading…' : 'Load older events'}
        </button>
      </div>
    {/if}
  </div>
</div>
