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
  import * as m from '$lib/i18n/messages';

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
      (!entryQuery.loading && !entryQuery.data?.admin ? m['admin.event_log.unavailable']() : null)
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

<PageTitle title={m['admin.event_log.event_page_title']({ sequence })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.event_log.event_title']({ sequence })}
    subtitle={m['admin.event_log.event_subtitle']()}
    {backHref}
    showMobileNav
  />

  <div class="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">{m['admin.event_log.loading_event']()}</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else if !entry}
      <Hint tone="warning">{m['admin.event_log.not_found']({ sequence })}</Hint>
    {:else}
      <Panel title={m['admin.event_log.metadata']()}>
        <dl class="grid grid-cols-1 gap-3 sm:grid-cols-[max-content_1fr] sm:gap-x-6">
          <dt class="text-sm text-muted">{m['admin.event_log.stream_sequence']()}</dt>
          <dd class="font-mono text-sm">{entry.sequence}</dd>

          <dt class="text-sm text-muted">{m['admin.event_log.subject']()}</dt>
          <dd class="font-mono text-sm">{entry.subject}</dd>

          <dt class="text-sm text-muted">{m['admin.event_log.event_type']()}</dt>
          <dd><Pill tone="accent">{entry.eventType || '—'}</Pill></dd>

          <dt class="text-sm text-muted">{m['admin.event_log.aggregate']()}</dt>
          <dd class="font-mono text-sm">
            {#if entry.aggregateType}
              <span class="text-muted">{entry.aggregateType}.</span>{entry.aggregateId}
            {:else}
              <span class="text-muted">—</span>
            {/if}
          </dd>

          <dt class="text-sm text-muted">{m['admin.event_log.event_id']()}</dt>
          <dd class="font-mono text-sm">{entry.eventId || '—'}</dd>

          <dt class="text-sm text-muted">{m['admin.event_log.actor']()}</dt>
          <dd class="font-mono text-sm">{entry.actorId || '—'}</dd>

          <dt class="text-sm text-muted">{m['admin.event_log.created_at']()}</dt>
          <dd class="text-sm">{formatTimestamp(entry.createdAt)}</dd>
        </dl>
      </Panel>

      <Panel title={m['admin.event_log.payload']()}>
        <pre
          class="overflow-x-auto rounded-md bg-surface-200 p-4 font-mono text-xs leading-relaxed">{entry.payloadJson}</pre>
      </Panel>
    {/if}
  </div>
</div>
