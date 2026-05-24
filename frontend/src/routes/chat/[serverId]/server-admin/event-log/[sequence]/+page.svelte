<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDateTime as formatDateTimeUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();

  const EventLogEntryQuery = graphql(`
    query AdminEventLogEntry($sequence: String!) {
      admin {
        eventLogEntry(sequence: $sequence) {
          sequence
          subject
          aggregateType
          aggregateId
          eventType
          eventId
          actorId
          createdAt
          payloadJson
        }
      }
    }
  `);

  const sequence = $derived(page.params.sequence!);

  const entryQuery = useQuery(EventLogEntryQuery, () => ({ sequence }));

  let entry = $derived(entryQuery.data?.admin?.eventLogEntry ?? null);
  let loading = $derived(entryQuery.loading);
  let error = $derived(
    entryQuery.error ??
      (!entryQuery.loading && !entryQuery.data?.admin
        ? 'Event log unavailable (admin access required)'
        : null)
  );

  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/event-log', {
      serverId: serverIdToSegment(getActiveServer())
    })
  );

  function formatTimestamp(iso: string): string {
    return formatDateTimeUtil(iso, userSettings);
  }
</script>

<PageTitle title="Event {sequence} | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Event {sequence}" subtitle="Single entry from EVT" backHref={backHref} showMobileNav />

  <div class="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading event…</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else if !entry}
      <Hint tone="warning">No event found at sequence {sequence}.</Hint>
    {:else}
      <Panel title="Metadata">
        <dl class="grid grid-cols-1 gap-3 sm:grid-cols-[max-content_1fr] sm:gap-x-6">
          <dt class="text-sm text-muted">Stream sequence</dt>
          <dd class="font-mono text-sm">{entry.sequence}</dd>

          <dt class="text-sm text-muted">Subject</dt>
          <dd class="font-mono text-sm">{entry.subject}</dd>

          <dt class="text-sm text-muted">Event type</dt>
          <dd><Pill tone="accent">{entry.eventType || '—'}</Pill></dd>

          <dt class="text-sm text-muted">Aggregate</dt>
          <dd class="font-mono text-sm">
            {#if entry.aggregateType}
              <span class="text-muted">{entry.aggregateType}.</span>{entry.aggregateId}
            {:else}
              <span class="text-muted">—</span>
            {/if}
          </dd>

          <dt class="text-sm text-muted">Event ID</dt>
          <dd class="font-mono text-sm">{entry.eventId || '—'}</dd>

          <dt class="text-sm text-muted">Actor</dt>
          <dd class="font-mono text-sm">{entry.actorId || '—'}</dd>

          <dt class="text-sm text-muted">Created at</dt>
          <dd class="text-sm">{formatTimestamp(entry.createdAt)}</dd>
        </dl>
      </Panel>

      <Panel title="Payload">
        <pre
          class="overflow-x-auto rounded-md bg-surface-200 p-4 font-mono text-xs leading-relaxed">{entry.payloadJson}</pre>
      </Panel>
    {/if}
  </div>
</div>
