<!-- @component Safe metadata shared by the latest failure and retained history. -->
<script lang="ts">
  import type { BotWebhookFailure } from '@chatto/api-types/api/v1/bots_pb';
  import { m } from '$lib/i18n/messages';

  let { failure }: { failure: BotWebhookFailure } = $props();
</script>

<div class="flex flex-wrap gap-x-4 gap-y-1 text-muted">
  <span>{m('settings.bots.outbound.attempts', { attempts: failure.attempts })}</span>
  {#if failure.httpStatus}
    <span>{m('settings.bots.outbound.http_status', { status: failure.httpStatus })}</span>
  {/if}
  {#if failure.reason}
    <span>{m('settings.bots.outbound.reason', { reason: failure.reason })}</span>
  {/if}
</div>
