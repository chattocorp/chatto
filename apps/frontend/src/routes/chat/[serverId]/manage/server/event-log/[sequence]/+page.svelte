<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createAdminEventLogAPI } from '$lib/api/adminEventLog';
  import { Panel, Hint, PaneContent, Pill, LoadingFog, PaneHeader, PageTitle } from '$lib/ui';
  import { JsonCode } from '$lib/ui/code';
  import {
    formatDateTime as formatDateTimeUtil,
    timeFormatSettingsFor
  } from '$lib/utils/formatTime';
  import { m } from '$lib/i18n/messages';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );

  const sequence = $derived(page.params.sequence!);
  const activeServerId = serverScope.serverId;
  const entryQuery = createQuery(() => {
    const serverId = activeServerId;
    const activeConnection = serverScope.connection;
    const eventSequence = sequence;
    return {
      queryKey: adminQueryKeys.event(serverId, activeConnection, eventSequence),
      queryFn: ({ signal }) =>
        activeConnection.getAPI(createAdminEventLogAPI).getEvent(eventSequence, { signal })
    };
  });

  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/event-log', {
      serverId: serverIdToSegment(activeServerId)
    })
  );

  function formatTimestamp(iso: string): string {
    return formatDateTimeUtil(iso, userSettings);
  }
</script>

<PageTitle title={m('admin.event_log.event_page_title', { sequence })} />

<div class="pane-page">
  <PaneHeader
    title={m('admin.event_log.event_title', { sequence })}
    subtitle={m('admin.event_log.event_subtitle')}
    {backHref}
  />

  <PaneContent>
    <div class="flex min-h-0 flex-col gap-6">
      {#if entryQuery.isPending}
        <LoadingFog class="h-40 w-full" label={m('admin.event_log.loading_event')} />
      {:else if entryQuery.error}
        <Hint tone="danger">
          {errorMessage(entryQuery.error, m('admin.event_log.unavailable'))}
        </Hint>
      {:else if !entryQuery.data}
        <Hint tone="warning">{m('admin.event_log.not_found', { sequence })}</Hint>
      {:else}
        {@const entry = entryQuery.data}
        <Panel title={m('admin.event_log.metadata')}>
          <dl class="grid grid-cols-1 gap-3 sm:grid-cols-[max-content_1fr] sm:gap-x-6">
            <dt class="text-sm text-muted">{m('admin.event_log.stream_sequence')}</dt>
            <dd class="font-mono text-sm">{entry.sequence}</dd>

            <dt class="text-sm text-muted">{m('admin.event_log.subject')}</dt>
            <dd class="font-mono text-sm">{entry.subject}</dd>

            <dt class="text-sm text-muted">{m('admin.event_log.event_type')}</dt>
            <dd><Pill tone="action">{entry.eventType || '—'}</Pill></dd>

            <dt class="text-sm text-muted">{m('admin.event_log.aggregate')}</dt>
            <dd class="font-mono text-sm">
              {#if entry.aggregateType}
                <span class="text-muted">{entry.aggregateType}.</span>{entry.aggregateId}
              {:else}
                <span class="text-muted">—</span>
              {/if}
            </dd>

            <dt class="text-sm text-muted">{m('admin.event_log.event_id')}</dt>
            <dd class="font-mono text-sm">{entry.eventId || '—'}</dd>

            <dt class="text-sm text-muted">{m('admin.event_log.actor')}</dt>
            <dd class="font-mono text-sm">{entry.actorId || '—'}</dd>

            <dt class="text-sm text-muted">{m('admin.event_log.created_at')}</dt>
            <dd class="text-sm">{formatTimestamp(entry.createdAt)}</dd>
          </dl>
        </Panel>

        <Panel title={m('admin.event_log.payload')}>
          <JsonCode text={entry.payloadJson} />
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
