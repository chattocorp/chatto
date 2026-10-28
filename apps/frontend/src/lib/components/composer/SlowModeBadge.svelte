<!-- @component Shows only an active posting cooldown. The parent owns the clock and sending rules. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { formatSlowModeCountdown } from '$lib/slowMode';
  import { HelpTooltip } from '$lib/ui';
  import ComposerStatusBadge from './ComposerStatusBadge.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  let {
    seconds,
    remaining = 0,
    bypassed = false
  }: { seconds: number; remaining?: number; bypassed?: boolean } = $props();
  const waiting = $derived(seconds > 0 && !bypassed && remaining > 0);
  const countdown = $derived(formatSlowModeCountdown(remaining));
  const description = $derived(m('composer.slow_mode_waiting', { countdown }));
  let surface: HTMLDivElement | undefined = $state();

  /** Ignore further attempts until this short feedback animation finishes. */
  export function highlight(): void {
    const badge = surface?.querySelector('.cooldown-badge');
    if (
      !waiting ||
      !badge ||
      badge.getAnimations().some((animation) => animation.playState === 'running')
    )
      return;
    const offsets = userPreferences.reducedMotion ? [0, 0, 0, 0, 0] : [0, -3, 3, -1, 0];
    badge.animate(
      offsets.map((offset, index) => ({
        transform: `translateX(${offset}px)`,
        backgroundColor:
          index === 0 || index === 4
            ? 'var(--color-background)'
            : 'color-mix(in srgb, var(--color-action) 25%, var(--color-background))'
      })),
      { duration: userPreferences.animations === 'disabled' ? 0 : 300, easing: 'ease-out' }
    );
  }
</script>

{#if waiting}
  <div bind:this={surface} class="pointer-events-auto shrink-0">
    <HelpTooltip
      label={description}
      triggerClass="flex cursor-help rounded-full focus-visible:outline-2 focus-visible:outline-action"
    >
      {#snippet triggerContent()}
        <ComposerStatusBadge
          data-testid="slow-mode-status"
          aria-label={description}
          class="cooldown-badge"
        >
          <span class="iconify icon-[uil--hourglass]" aria-hidden="true"></span>
          <span class="tabular-nums" dir="ltr">{countdown}</span>
        </ComposerStatusBadge>
      {/snippet}
      {description}
    </HelpTooltip>
  </div>
{/if}
