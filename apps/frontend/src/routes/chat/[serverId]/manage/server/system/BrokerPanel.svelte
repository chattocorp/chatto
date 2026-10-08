<!--
@component

Shows the NATS connection of the Chatto replica that handled the diagnostics
request. With more than one replica, other replicas can connect to a
different NATS node.
-->
<script lang="ts">
  import type { AdminConnectionInfo } from '$lib/api/adminDiagnostics';
  import { formatBytes, formatGoDuration } from '$lib/components/admin';
  import { HelpTooltip, Panel } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let { connection }: { connection: AdminConnectionInfo } = $props();
</script>

<Panel title={m('admin.system.broker')} icon="iconify icon-[uil--server]">
  <div class="flex flex-col gap-5">
    <div class="flex items-center justify-between gap-3">
      <div class="flex items-center gap-2 text-lg font-semibold">
        <span
          aria-hidden="true"
          class={['h-2.5 w-2.5 rounded-full', connection.connected ? 'bg-success' : 'bg-danger']}
        ></span>
        {connection.connected ? m('admin.system.connected') : m('admin.system.disconnected')}
      </div>
      <HelpTooltip label={m('admin.system.broker_scope_label')}>
        {m('admin.system.broker_scope_help')}
      </HelpTooltip>
    </div>

    <dl class="grid grid-cols-2 gap-x-6 gap-y-3">
      <div>
        <dt class="text-sm text-muted">{m('admin.common.version')}</dt>
        <dd class="font-mono text-sm">{connection.version || '-'}</dd>
      </div>
      <div class="min-w-0">
        <dt class="text-sm text-muted">{m('admin.system.server_name')}</dt>
        <dd class="truncate font-mono text-sm" title={connection.serverName}>
          {connection.serverName || '-'}
        </dd>
      </div>
      <div>
        <dt class="text-sm text-muted">{m('admin.system.rtt')}</dt>
        <dd class="font-mono text-sm" title={connection.rtt}>
          {connection.rtt ? formatGoDuration(connection.rtt) : '-'}
        </dd>
      </div>
      <div>
        <dt class="text-sm text-muted">{m('admin.system.max_payload')}</dt>
        <dd class="font-mono text-sm">{formatBytes(connection.maxPayload)}</dd>
      </div>
      <div class="col-span-2 min-w-0">
        <dt class="text-sm text-muted">{m('admin.system.server_id')}</dt>
        <dd class="truncate font-mono text-sm" title={connection.serverId}>
          {connection.serverId || '-'}
        </dd>
      </div>
    </dl>
  </div>
</Panel>
