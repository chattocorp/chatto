<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    eventId,
    body,
    echoedToChannel = false,
    oncontextmenu,
    onmousedown,
    onmouseup,
    onmouseleave,
    ontouchstart,
    ontouchend,
    ontouchmove,
    ontouchcancel,
    bodyElement = $bindable(),
    afterBody,
    actions
  }: {
    eventId: string;
    body?: string | null;
    echoedToChannel?: boolean;
    oncontextmenu?: (event: MouseEvent) => void;
    onmousedown?: (event: MouseEvent) => void;
    onmouseup?: (event: MouseEvent) => void;
    onmouseleave?: (event: MouseEvent) => void;
    ontouchstart?: (event: TouchEvent) => void;
    ontouchend?: (event: TouchEvent) => void;
    ontouchmove?: (event: TouchEvent) => void;
    ontouchcancel?: (event: TouchEvent) => void;
    bodyElement?: HTMLElement;
    afterBody?: Snippet;
    actions?: Snippet;
  } = $props();
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="message-row"
  data-testid="message-row"
  data-event-id={eventId}
  {oncontextmenu}
  {onmousedown}
  {onmouseup}
  {onmouseleave}
  {ontouchstart}
  {ontouchend}
  {ontouchmove}
  {ontouchcancel}
>
  <span data-testid="message-body" bind:this={bodyElement}>{body}</span>
  {#if echoedToChannel}
    <span class="echoed-to-channel-marker" role="img" aria-label="Also sent to channel"></span>
  {/if}
  {@render afterBody?.()}
  {@render actions?.()}
</div>
