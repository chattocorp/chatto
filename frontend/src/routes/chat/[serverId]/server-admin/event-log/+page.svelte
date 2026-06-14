<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import type { AdminEventLogQuery } from '$lib/gql/graphql';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { formatDateTime as formatDateTimeUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();
  const connection = useConnection();

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

  const pageSize = 50;
  type EventLogConnection = NonNullable<AdminEventLogQuery['admin']>['eventLog'];
  type Entry = EventLogConnection['entries'][number];

  let entries = $state<Entry[]>([]);
  let totalCount = $state('0');
  let hasOlder = $state(false);
  let endCursor = $state<string | null>(null);
  let loading = $state(true);
  let loadingMore = $state(false);
  let error = $state<string | null>(null);
  let requestId = 0;
  let scrollContainer = $state<HTMLDivElement>();
  let formattedTotalCount = $derived(formatTotalCount(totalCount));

  onMount(() => {
    void loadFirstPage();
  });

  function formatTotalCount(count: string): string {
    const numeric = Number(count);
    return Number.isSafeInteger(numeric) ? numeric.toLocaleString() : count;
  }

  function formatTimestamp(iso: string): string {
    return formatDateTimeUtil(iso, userSettings);
  }

  async function queryEventLog(before: string | null) {
    return connection()
      .client.query(EventLogQuery, {
        limit: pageSize,
        before
      })
      .toPromise();
  }

  async function loadFirstPage() {
    const currentRequest = ++requestId;
    loading = true;
    error = null;
    entries = [];
    totalCount = '0';
    hasOlder = false;
    endCursor = null;

    try {
      const result = await queryEventLog(null);
      if (currentRequest !== requestId) return;

      if (result.error) {
        error = result.error.message;
        return;
      }

      const conn = result.data?.admin?.eventLog;
      if (!conn) {
        error = 'Event log unavailable (audit permission required)';
        return;
      }

      entries = conn.entries;
      totalCount = String(conn.totalCount);
      hasOlder = conn.hasOlder;
      endCursor = conn.endCursor ?? null;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load event log';
    } finally {
      if (currentRequest === requestId) {
        loading = false;
      }
    }
  }

  async function loadMore() {
    if (loading || loadingMore || !hasOlder) return;

    const before = endCursor ?? entries[entries.length - 1]?.sequence;
    if (!before) return;

    const currentRequest = ++requestId;
    loadingMore = true;
    error = null;

    try {
      const result = await queryEventLog(before);
      if (currentRequest !== requestId) return;

      if (result.error) {
        error = result.error.message;
        return;
      }

      const conn = result.data?.admin?.eventLog;
      if (!conn) {
        error = 'Event log unavailable (audit permission required)';
        return;
      }

      entries = mergeEntries(entries, conn.entries);
      totalCount = String(conn.totalCount);
      hasOlder = conn.hasOlder;
      endCursor = conn.endCursor ?? null;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load older events';
    } finally {
      if (currentRequest === requestId) {
        loadingMore = false;
      }
    }
  }

  function mergeEntries(existing: Entry[], next: Entry[]): Entry[] {
    const seen = new Set(existing.map((entry) => entry.sequence));
    return [...existing, ...next.filter((entry) => !seen.has(entry.sequence))];
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

  <div class="min-h-0 flex-1 overflow-y-auto" bind:this={scrollContainer}>
    <div class="flex flex-col gap-4 p-6">
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {/if}

      <div class="text-sm text-muted">
        {formattedTotalCount} total event{totalCount === '1' ? '' : 's'} in stream
      </div>

      <Panel noPadding>
        <DataTable
          items={entries}
          columns={5}
          emptyMessage={loading ? 'Loading…' : 'No events recorded yet.'}
          hasMore={hasOlder && !error}
          {loadingMore}
          onLoadMore={loadMore}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage="Loading older events..."
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
    </div>
  </div>
</div>
