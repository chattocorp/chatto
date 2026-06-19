<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import {
    GetAdminEventLogEntryRequest,
    type AdminEventLogEntryView
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';
  import { formatDateTime as formatDateTimeUtil } from '$lib/utils/formatTime';

  const userSettings = getUserSettings();

  type Entry = {
    sequence: string;
    subject: string;
    aggregateType: string;
    aggregateId: string;
    eventType: string;
    eventId: string;
    actorId: string;
    createdAt: string;
    payloadJson: string;
  };

  const sequence = $derived(page.params.sequence!);

  let entry = $state<Entry | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(() => {
    void loadEntry(sequence);
  });

  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/event-log', {
      serverId: serverIdToSegment(getActiveServer())
    })
  );

  function formatTimestamp(iso: string): string {
    if (!iso) return '-';
    return formatDateTimeUtil(iso, userSettings);
  }

  async function loadEntry(targetSequence: string) {
    loading = true;
    error = null;
    entry = null;

    try {
      const response = await withActiveServerWireClient((client) =>
        client.getAdminEventLogEntry(new GetAdminEventLogEntryRequest({ sequence: targetSequence }))
      );
      entry = response.entry ? mapEventLogEntry(response.entry) : null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load event';
      console.error('Failed to load event log entry', err);
    } finally {
      loading = false;
    }
  }

  function mapEventLogEntry(value: AdminEventLogEntryView): Entry {
    return {
      sequence: value.sequence,
      subject: value.subject,
      aggregateType: value.aggregateType,
      aggregateId: value.aggregateId,
      eventType: value.eventType,
      eventId: value.eventId,
      actorId: value.actorId,
      createdAt: value.createdAt?.toDate().toISOString() ?? '',
      payloadJson: value.payloadJson
    };
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
