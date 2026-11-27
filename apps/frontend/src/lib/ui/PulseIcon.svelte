<!--
@component

An icon glyph with an optional ping pulse behind it, for live state such as
an active call. The pulse is a faded copy of the glyph that grows and fades
out. Both use the current text colour; pass a colour through `class`. The
icon is decorative: the owner names the state, for example with an
`aria-label` on its container.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';

  let {
    icon,
    pulse = true,
    class: className
  }: {
    /** Iconify class name, such as `icon-[uil--phone]`. */
    icon: string;
    /** Whether the pulse runs. Turn it off to keep the same glyph without motion. */
    pulse?: boolean;
    /** Colour or layout classes for the glyph and its pulse. */
    class?: ClassValue;
  } = $props();
</script>

<span class={['relative inline-flex', className]} aria-hidden="true">
  {#if pulse}
    <span
      class={['absolute inset-0 pane-header-icon-glyph animate-ping opacity-45', icon]}
      data-testid="active-call-pulse-icon"
    ></span>
  {/if}
  <span class={['relative pane-header-icon-glyph', icon]}></span>
</span>
