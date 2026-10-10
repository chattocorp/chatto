<!--
@component

Shows the LiveKit voice and video call setup: the result of the server's
LiveKit API check, setup warnings, and the values that the LiveKit
configuration must match. Webhook times come from the Chatto server that
handled the request.
-->
<script lang="ts">
  import type { AdminLiveKitStatus } from '$lib/api/adminDiagnostics';
  import { Hint, Panel, Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { liveKitWebhooksRejected } from './systemHealth';

  let { status }: { status: AdminLiveKitStatus } = $props();

  /** TURN setup lives in LiveKit, so Chatto can only point to the guide. */
  const TURN_GUIDE_URL =
    'https://docs.chatto.run/guides/infrastructure/voice-calls/#turn-for-restrictive-networks';

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );

  const tone = $derived(
    status.connectionState === 'ok'
      ? 'success'
      : status.connectionState === 'not_configured'
        ? 'muted'
        : 'danger'
  );
  const stateLabel = $derived(
    status.connectionState === 'ok'
      ? m('admin.system.livekit.state_ok')
      : status.connectionState === 'not_configured'
        ? m('admin.system.livekit.state_not_configured')
        : status.connectionState === 'unreachable'
          ? m('admin.system.livekit.state_unreachable')
          : status.connectionState === 'unauthorized'
            ? m('admin.system.livekit.state_unauthorized')
            : m('admin.system.livekit.state_error')
  );
  const summary = $derived(
    status.connectionState === 'ok'
      ? m('admin.system.livekit.summary_ok')
      : status.connectionState === 'not_configured'
        ? m('admin.system.livekit.summary_not_configured')
        : status.connectionState === 'unreachable'
          ? m('admin.system.livekit.summary_unreachable')
          : status.connectionState === 'unauthorized'
            ? m('admin.system.livekit.summary_unauthorized')
            : m('admin.system.livekit.summary_error')
  );
  const webhooksRejected = $derived(liveKitWebhooksRejected(status));

  function formatTimestamp(value: Date | null): string {
    return value
      ? formatDateTime(value, userSettings, getLocale())
      : m('admin.system.livekit.webhook_none');
  }
</script>

<Panel title={m('admin.system.livekit.title')} icon="iconify icon-[uil--video]">
  <div class="flex flex-col gap-4" data-testid="livekit-panel">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="text-sm text-muted">{m('admin.common.status')}</div>
        <div class="mt-1"><Pill {tone}>{stateLabel}</Pill></div>
      </div>
      <div class="max-w-2xl text-sm text-muted">{summary}</div>
    </div>

    {#if status.connectionError}
      <div class="font-mono text-xs break-words text-danger">{status.connectionError}</div>
    {/if}

    {#if status.configured}
      {#if status.insecureUrl}
        <Hint tone="warning">{m('admin.system.livekit.insecure_url')}</Hint>
      {/if}
      {#if webhooksRejected}
        <Hint tone="warning">{m('admin.system.livekit.webhooks_rejected')}</Hint>
      {:else if !status.lastWebhookAt}
        <Hint>{m('admin.system.livekit.no_webhooks')}</Hint>
      {/if}

      <dl class="grid gap-x-6 gap-y-4 md:grid-cols-2">
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.url')}</dt>
          <dd class="font-mono text-sm break-all" dir="ltr">{status.url}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.api_key')}</dt>
          <dd class="font-mono text-sm break-all" dir="ltr">{status.apiKey}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.webhook_url')}</dt>
          <dd class="font-mono text-sm break-all" dir="ltr">{status.webhookUrl || '-'}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.webhook_key')}</dt>
          <dd class="text-sm">
            {status.separateWebhookKey
              ? m('admin.system.livekit.webhook_key_separate')
              : m('admin.system.livekit.webhook_key_same')}
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.last_webhook')}</dt>
          <dd class="text-sm">{formatTimestamp(status.lastWebhookAt)}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-sm text-muted">{m('admin.system.livekit.last_rejected_webhook')}</dt>
          <dd class={['text-sm', webhooksRejected ? 'text-warning' : '']}>
            {formatTimestamp(status.lastRejectedWebhookAt)}
          </dd>
        </div>
      </dl>

      <p class="text-sm text-muted">
        {m('admin.system.livekit.scope_help')}
        <a href={TURN_GUIDE_URL} target="_blank" rel="noopener noreferrer" class="link">
          {m('admin.system.livekit.turn_guide')}
        </a>
      </p>
    {/if}
  </div>
</Panel>
