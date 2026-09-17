<!-- @component Network-only warning shared by call participant and current-user cards. Healthy and unknown quality stay quiet. -->
<script lang="ts">
  import { CompactActionButton, FloatingPopover } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import type { CallParticipantInfo } from '$lib/state/server/voiceCall.svelte';

  let { quality }: { quality?: CallParticipantInfo['connectionQuality'] } = $props();
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  const warning = $derived(quality === 'poor' || quality === 'lost');
  const label = $derived(
    quality === 'lost' ? m('voice.connection_lost') : m('voice.poor_connection')
  );
  // Removing the warning also discards its open explanation.
  function resetOnRemove() {
    return () => {
      anchor = null;
    };
  }
  function toggle(event: MouseEvent) {
    event.stopPropagation();
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    anchor = anchor ? null : { top: rect.top, bottom: rect.bottom, left: rect.left };
  }
</script>

<svelte:document
  onkeydown={(event) => {
    if (event.key === 'Escape') anchor = null;
  }}
/>

{#if warning}
  <div class="contents" {@attach resetOnRemove}>
    <CompactActionButton
      {label}
      class={quality === 'lost' ? 'text-danger' : 'text-warning'}
      data-testid="call-connection-quality"
      aria-haspopup="dialog"
      aria-expanded={!!anchor}
      onclick={toggle}
    >
      <span
        class={[
          'iconify',
          quality === 'lost' ? 'icon-[mdi--wifi-off]' : 'icon-[mdi--wifi-strength-1]'
        ]}
        aria-hidden="true"
      ></span>
    </CompactActionButton>
    {#if anchor}
      <FloatingPopover
        {anchor}
        role="dialog"
        ariaLabel={label}
        class="menu"
        onclose={() => (anchor = null)}
      >
        <div class="menu-section px-3 py-2 text-sm">{label}</div>
      </FloatingPopover>
    {/if}
  </div>
{/if}
