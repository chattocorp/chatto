<!--
@component
Reply and edit context inside the message composer. The text follows the
draft's inset, with a quiet cancel control for pointer and keyboard users.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import type { AccountNameIdentity } from '$lib/render/accountName';
  import { m } from '$lib/i18n/messages';
  import { CompactActionButton } from '$lib/ui';

  let {
    inReplyTo,
    replyDisplayName,
    replyIdentity,
    replyExcerpt,
    isEditing,
    expandedDraft,
    oncancelreply,
    oncanceledit
  }: {
    inReplyTo?: string;
    replyDisplayName?: string;
    replyIdentity?: AccountNameIdentity;
    replyExcerpt?: string;
    isEditing: boolean;
    /** Match the draft's full-width layout when it grows beyond one line. */
    expandedDraft: boolean;
    oncancelreply: () => void;
    oncanceledit: () => void;
  } = $props();
</script>

{#if inReplyTo && replyDisplayName}
  <div
    data-testid="reply-indicator"
    class={[
      'relative flex min-w-0 items-center gap-2 ps-0.5 text-xs text-muted',
      !expandedDraft && '@min-[560px]/composer:ps-8.5'
    ]}
  >
    <span
      aria-hidden="true"
      class={[
        'iconify absolute start-1 icon-[uil--corner-up-left] hidden rtl:-scale-x-100',
        !expandedDraft && '@min-[560px]/composer:block'
      ]}
    ></span>
    <span class="min-w-0 flex-1 truncate">
      {m('composer.replying_to')}
      <strong class="inline-flex max-w-full min-w-0 font-semibold"
        ><AccountName name={replyDisplayName} identity={replyIdentity} /></strong
      >
      {#if replyExcerpt}
        <bdi> · {replyExcerpt}</bdi>
      {/if}
    </span>
    {@render cancel(oncancelreply)}
  </div>
{/if}

{#if isEditing}
  <div
    data-testid="edit-indicator"
    class={[
      'relative flex min-w-0 items-center gap-2 ps-0.5 text-xs text-muted',
      !expandedDraft && '@min-[560px]/composer:ps-8.5'
    ]}
  >
    <span
      aria-hidden="true"
      class={[
        'iconify absolute start-1 icon-[uil--pen] hidden',
        !expandedDraft && '@min-[560px]/composer:block'
      ]}
    ></span>
    <span class="min-w-0 flex-1">{m('composer.editing')}</span>
    {@render cancel(oncanceledit)}
  </div>
{/if}

{#snippet cancel(onclick: () => void)}
  <CompactActionButton
    label={m('common.cancel')}
    title={`Esc ${m('composer.esc_to_cancel')}`}
    {onclick}
    touchFriendly
  >
    <span aria-hidden="true" class="iconify icon-[uil--times]"></span>
  </CompactActionButton>
{/snippet}
