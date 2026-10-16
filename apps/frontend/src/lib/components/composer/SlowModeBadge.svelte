<!-- @component Shows only an active posting cooldown. The parent owns the clock and sending rules. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { formatSlowModeCountdown } from '$lib/slowMode';
  import { HelpTooltip } from '$lib/ui';
  import ComposerStatusBadge from './ComposerStatusBadge.svelte';
  let {
    seconds,
    remaining = 0,
    bypassed = false
  }: { seconds: number; remaining?: number; bypassed?: boolean } = $props();
  const waiting = $derived(seconds > 0 && !bypassed && remaining > 0);
  const countdown = $derived(formatSlowModeCountdown(remaining));
  const description = $derived(m('composer.slow_mode_waiting', { countdown }));
</script>

{#if waiting}
  <div class="pointer-events-auto shrink-0">
    <HelpTooltip
      label={description}
      triggerClass="flex cursor-help rounded-full focus-visible:outline-2 focus-visible:outline-action"
    >
      {#snippet triggerContent()}
        <ComposerStatusBadge data-testid="slow-mode-status" aria-label={description}>
          <span class="iconify icon-[uil--hourglass]" aria-hidden="true"></span>
          <span class="tabular-nums" dir="ltr">{countdown}</span>
        </ComposerStatusBadge>
      {/snippet}
      {description}
    </HelpTooltip>
  </div>
{/if}
