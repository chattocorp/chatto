<!--
@component

Two-line text body for an `ActivityListRow`: a title line with the time at its
end, and an optional muted subtitle line with optional trailing metadata.
`emphasized` renders the title in bold for unread activity; other rows keep
regular weight in the normal text color, so read rows stay readable.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    time,
    trailing,
    emphasized = false,
    wrapOnNarrow = false,
    testId
  }: {
    title: Snippet;
    subtitle?: Snippet;
    /** Formatted time shown at the end of the title line. */
    time: string;
    /** Short metadata at the end of the subtitle line, such as a count. */
    trailing?: string;
    emphasized?: boolean;
    /** Wrap instead of truncate below 768px, for lists that must stay fully readable. */
    wrapOnNarrow?: boolean;
    testId?: string;
  } = $props();

  const overflow = $derived(wrapOnNarrow ? 'max-md:wrap-anywhere md:truncate' : 'truncate');
</script>

<span class="min-w-0 flex-1" data-testid={testId}>
  <span class="flex min-w-0 items-baseline gap-2">
    <bdi
      class={['min-w-0 flex-1', overflow, emphasized ? 'font-semibold text-text-top' : 'text-text']}
      dir="auto"
    >
      {@render title()}
    </bdi>
    <span class="shrink-0 text-sm text-muted tabular-nums">{time}</span>
  </span>
  {#if subtitle || trailing}
    <span class="flex min-w-0 items-baseline gap-2 text-sm text-muted">
      <bdi class={['min-w-0 flex-1', overflow]} dir="auto">
        {#if subtitle}{@render subtitle()}{/if}
      </bdi>
      {#if trailing}<span class="shrink-0">{trailing}</span>{/if}
    </span>
  {/if}
</span>
