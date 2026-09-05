<!--
@component

Renders one message search result with shared identity and attachment details.
The owner supplies metadata, optional preview layout, and result navigation.
Enter activates the result only when the result itself has focus.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue, HTMLAttributes } from 'svelte/elements';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import type { TimeFormatSettings } from '$lib/utils/formatTime';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    result,
    viewerLogin,
    timestampSettings,
    timestampLocale,
    headerMeta,
    preview,
    attachmentClass = 'text-sm',
    onOpen,
    class: className,
    ...attributes
  }: Omit<HTMLAttributes<HTMLDivElement>, 'children' | 'onclick' | 'onkeydown'> & {
    result: MessageSearchResult;
    viewerLogin?: string;
    timestampSettings: TimeFormatSettings;
    timestampLocale: string;
    headerMeta: Snippet;
    /** Optional wrapper, such as an inert preview with a height limit. */
    preview?: Snippet<[Snippet]>;
    attachmentClass?: ClassValue;
    onOpen: (result: MessageSearchResult) => void;
  } = $props();

  const actor = $derived(
    result.actor ? { ...result.actor, presenceStatus: PresenceStatus.OFFLINE } : null
  );
  const displayName = $derived(
    result.actor?.displayName || result.actor?.login || m('common.unknown')
  );

  function open(event: MouseEvent): void {
    // The whole result is one target, including links within the preview.
    event.preventDefault();
    onOpen(result);
  }

  function openFromKeyboard(event: KeyboardEvent): void {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    onOpen(result);
  }
</script>

{#snippet message()}
  <MessageView
    eventId={result.id}
    {actor}
    {displayName}
    missingActorIsDeleted={false}
    body={result.body}
    {viewerLogin}
    {timestampSettings}
    {timestampLocale}
    {headerMeta}
    rowClass="hover:bg-transparent md:mx-0 md:pe-2"
  >
    {#snippet afterBody()}
      {#if result.attachmentCount > 0}
        <p class={['inline-flex items-center gap-1 text-muted', attachmentClass]}>
          <span class="iconify icon-[uil--paperclip]" aria-hidden="true"></span>
          {m('search.attachments', { count: result.attachmentCount })}
        </p>
      {/if}
    {/snippet}
  </MessageView>
{/snippet}

<div
  {...attributes}
  role="link"
  tabindex="0"
  class={['cursor-pointer selectable-list-item', className]}
  onclick={open}
  onkeydown={openFromKeyboard}
>
  {#if preview}
    {@render preview(message)}
  {:else}
    {@render message()}
  {/if}
</div>
