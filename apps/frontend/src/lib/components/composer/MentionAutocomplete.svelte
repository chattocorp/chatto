<!--
@component

Discord-style @mention autocomplete popup.
Shows matching room members when typing @username in chat input.

**Props:**
- `query` - Current search query (without the leading @)
- `members` - Room members to search through
- `roles` - Mentionable roles
- `prioritizedUserIds` - Users that rank first among matches, such as thread participants
- `onSelect` - Callback when a member is selected (receives login and whether Tab was used)
- `onClose` - Callback to close the popup
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import type { RoomMember } from '$lib/state/room';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import AutocompletePopup from './AutocompletePopup.svelte';
  import type { MentionRole } from './autocomplete.svelte';
  import { rankMentionCandidates, type MentionResult } from './mentionRanking';
  import { m } from '$lib/i18n/messages';

  type Props = {
    query: string;
    members: RoomMember[];
    roles?: MentionRole[];
    /** Users that rank first among matches, such as thread participants. */
    prioritizedUserIds?: ReadonlySet<string>;
    onSelect: (handle: string, viaTab: boolean) => void;
    onClose: () => void;
  };

  let { query, members, roles = [], prioritizedUserIds, onSelect, onClose }: Props = $props();

  let results = $derived(
    rankMentionCandidates(query, members, roles, prioritizedUserIds).slice(0, 10)
  );

  let popupRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  export function handleKeyDown(event: KeyboardEvent): boolean {
    return popupRef?.handleKeyDown(event) ?? false;
  }

  function handleSelect(result: MentionResult, key: string) {
    onSelect(result.handle, key === 'Tab');
  }
</script>

<AutocompletePopup
  bind:this={popupRef}
  items={results}
  getKey={(r) => `${r.type}:${r.handle}`}
  selectKeys={['Enter', 'Tab']}
  onSelect={handleSelect}
  {onClose}
  testid="mention-autocomplete"
  class="md:w-72"
>
  {#snippet item({ item: result })}
    {#if result.type === 'user'}
      <UserAvatar user={result.member} size="xs" class="h-6 w-6" useLiveProfile={false} />
      <AccountName
        name={result.member.displayName}
        identity={result.member}
        class="text-sm text-text"
      />
      <bdi dir="ltr" class="min-w-0 truncate text-sm text-muted">@{result.member.login}</bdi>
    {:else if result.type === 'virtual'}
      <div
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-semibold text-muted"
      >
        <span aria-hidden="true" class="iconify icon-[uil--megaphone] h-4 w-4"></span>
      </div>
      <bdi class="min-w-0 truncate text-sm text-text">
        {result.handle === 'all'
          ? m('composer.mention.all_room_members')
          : m('composer.mention.members_here')}
      </bdi>
      <bdi dir="ltr" class="min-w-0 truncate text-sm text-muted">@{result.handle}</bdi>
    {:else}
      <div
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-semibold text-muted"
      >
        <span aria-hidden="true" class="iconify icon-[uil--users-alt] h-4 w-4"></span>
      </div>
      <span class="min-w-0 truncate text-sm text-text">{m('composer.mention.role')}</span>
      <bdi dir="ltr" class="min-w-0 truncate text-sm text-muted">@{result.role.name}</bdi>
    {/if}
  {/snippet}
</AutocompletePopup>
