<script lang="ts">
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import DeletedUserLabel from '$lib/components/DeletedUserLabel.svelte';
  import type { CallPresenceKind } from '$lib/state/server/activeCallRooms.svelte';
  import { m } from '$lib/i18n/messages';
  import type { MessageReplyPreview } from './messageEventModel';

  let {
    preview,
    compact = false,
    callPresence = null,
    onJump,
    onAuthorClick
  }: {
    preview: MessageReplyPreview;
    compact?: boolean;
    callPresence?: CallPresenceKind | null;
    onJump: () => void;
    onAuthorClick?: (event: MouseEvent) => void;
  } = $props();

  const jumpText = $derived(
    preview.body ??
      (preview.actor || preview.deleted
        ? m('room.message.meta.reply_preview_fallback')
        : preview.name)
  );
  const jumpLabel = $derived(`${m('room.message.meta.in_reply_to')} ${jumpText}`);
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  data-testid="reply-attribution"
  aria-label={m('room.message.meta.in_reply_to')}
  title={m('room.message.meta.in_reply_to')}
  class={[
    'group/reply relative flex min-w-0 cursor-pointer items-center gap-1.5 py-0.5 text-xs leading-none text-muted',
    compact ? '' : '-ms-[39px] ps-[39px]'
  ]}
  onclick={onJump}
  onmousedown={(event) => event.stopPropagation()}
>
  {#if compact}
    <span
      aria-hidden="true"
      class="rounded-ts-md h-3 w-5 shrink-0 border-s-2 border-t-2 border-surface-strong/30 transition-colors group-hover/reply:border-surface-strong/55"
    ></span>
  {:else}
    <span
      aria-hidden="true"
      class="rounded-ts-md absolute start-0 top-[11px] h-7 w-[39px] border-s-2 border-t-2 border-surface-strong/30 transition-colors group-hover/reply:border-surface-strong/55"
    ></span>
  {/if}

  {#if preview.actor}
    <button
      type="button"
      data-testid="reply-attribution-author"
      class="inline-flex max-w-[45%] min-w-0 shrink-0 cursor-pointer items-center gap-1 hover:underline"
      onclick={(event) => {
        event.stopPropagation();
        onAuthorClick?.(event);
      }}
    >
      <UserAvatar user={preview.actor} size="xs" />
      <strong class="truncate font-medium"><bdi>{preview.name}</bdi></strong>
      {#if callPresence}
        <span
          class={[
            'iconify shrink-0 text-xs leading-none text-action',
            callPresence === 'video' ? 'icon-[uil--video]' : 'icon-[uil--phone]'
          ]}
          title={callPresence === 'video' ? 'In a video call' : 'In a voice call'}
          aria-label={callPresence === 'video' ? 'In a video call' : 'In a voice call'}
          data-testid={`user-call-presence-${callPresence}`}
        ></span>
      {/if}
    </button>
  {:else if preview.deleted}
    <strong class="max-w-[45%] shrink-0 truncate font-medium"><DeletedUserLabel /></strong>
  {/if}

  <button
    type="button"
    class="min-w-0 flex-1 cursor-pointer truncate text-start opacity-75 hover:text-text"
    aria-label={jumpLabel}
    title={jumpLabel}
  >
    {jumpText}
  </button>
</div>
