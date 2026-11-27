<!--
@component

Grid of the main actions for a profile: open a conversation, mention the user in
the current composer, and search their messages. Each action shows only when
the host supplies its callback, so hosts decide availability and permissions.
The first available action uses the primary style.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';

  type QuickAction = {
    id: 'message' | 'mention' | 'search';
    icon: string;
    label: string;
    run: () => void;
  };

  let {
    onSendMessage,
    onMention,
    onSearchMessages,
    class: className
  }: {
    onSendMessage?: () => void;
    onMention?: () => void;
    onSearchMessages?: () => void;
    class?: string;
  } = $props();

  const actions = $derived(
    [
      onSendMessage && {
        id: 'message',
        icon: 'icon-[uil--comment-alt-message]',
        label: m('chat.user_menu.send_message'),
        run: onSendMessage
      },
      onMention && {
        id: 'mention',
        icon: 'icon-[uil--at]',
        label: m('chat.profile.actions.mention'),
        run: onMention
      },
      onSearchMessages && {
        id: 'search',
        icon: 'icon-[uil--search]',
        label: m('chat.profile.actions.search_messages'),
        run: onSearchMessages
      }
    ].filter((action): action is QuickAction => !!action)
  );
</script>

{#if actions.length}
  <!-- Two columns; with an odd count, the primary action takes the full first row. -->
  <div class={['grid grid-cols-2 gap-1.5', className]} data-testid="profile-quick-actions">
    {#each actions as action, index (action.id)}
      <button
        type="button"
        class={[
          'flex min-h-14 min-w-0 cursor-pointer flex-col items-center justify-center gap-1 rounded-md px-2 py-2 text-center text-sm leading-tight transition-[background-color,scale] feedback-quick active:scale-[0.96]',
          index === 0 && actions.length % 2 === 1 && 'col-span-2',
          index === 0
            ? 'bg-action text-on-action hover:bg-action-hover'
            : 'bg-surface text-text hover:bg-surface-emphasized'
        ]}
        data-testid={`profile-action-${action.id}`}
        onclick={action.run}
      >
        <span class={['iconify size-5 shrink-0', action.icon]} aria-hidden="true"></span>
        <span class="min-w-0 break-words">{action.label}</span>
      </button>
    {/each}
  </div>
{/if}
