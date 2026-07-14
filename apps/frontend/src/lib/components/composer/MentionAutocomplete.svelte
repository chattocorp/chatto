<!--
@component

Discord-style @mention autocomplete popup.
Shows matching room members when typing @username in chat input.

**Props:**
- `query` - Current search query (without the leading @)
- `members` - Room members to search through
- `onSelect` - Callback when a member is selected (receives login and whether Tab was used)
- `onClose` - Callback to close the popup
-->
<script lang="ts">
  import type { RoomMember } from '$lib/state/room';
  import { fuzzyMatch } from '$lib/fuzzyMatch';
  import { getAvatarInitials } from '$lib/utils/initials';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import AutocompletePopup from './AutocompletePopup.svelte';
  import type { MentionRole } from './autocomplete.svelte';
  import * as m from '$lib/i18n/messages';

  type MentionResult =
    | { type: 'user'; handle: string; member: RoomMember; score: number; priority: number }
    | { type: 'virtual'; handle: 'all' | 'here'; label: string; score: number; priority: number }
    | { type: 'role'; handle: string; role: MentionRole; score: number; priority: number };

  type Props = {
    query: string;
    members: RoomMember[];
    roles?: MentionRole[];
    onSelect: (handle: string, viaTab: boolean) => void;
    onClose: () => void;
  };

  let { query, members, roles = [], onSelect, onClose }: Props = $props();

  let results = $derived.by(() => {
    const scored: MentionResult[] = [];

    for (const m of members) {
      if (m.deleted || !m.login) continue;

      const loginScore = fuzzyMatch(query, m.login);
      const displayScore = fuzzyMatch(query, m.displayName);
      const bestScore = Math.max(loginScore ?? -1, displayScore ?? -1);

      if (bestScore > 0) {
        scored.push({ type: 'user', handle: m.login, member: m, score: bestScore, priority: 0 });
      }
    }

    for (const target of [
      { handle: 'all' as const, label: m['composer.mention.all_room_members']() },
      { handle: 'here' as const, label: m['composer.mention.members_here']() }
    ]) {
      const score = fuzzyMatch(query, target.handle);
      if (score && score > 0) {
        scored.push({ type: 'virtual', ...target, score, priority: 1 });
      }
    }

    for (const role of roles) {
      if (!role.pingable || role.name === 'everyone') continue;
      const score = fuzzyMatch(query, role.name);
      if (score && score > 0) {
        scored.push({ type: 'role', handle: role.name, role, score, priority: 2 });
      }
    }

    scored.sort(
      (a, b) => a.priority - b.priority || b.score - a.score || a.handle.localeCompare(b.handle)
    );
    return scored.slice(0, 10);
  });

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
      {#if result.member.avatarUrl}
        <SkeletonImg
          loading="lazy"
          src={result.member.avatarUrl}
          alt={result.member.login}
          class="h-6 w-6 shrink-0 rounded-full object-cover"
        />
      {:else}
        <div
          class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-semibold text-muted"
        >
          {getAvatarInitials(result.member.displayName, result.member.login)}
        </div>
      {/if}
      <span class="min-w-0 truncate text-sm text-text">{result.member.displayName}</span>
      <span class="min-w-0 truncate text-sm text-muted">@{result.member.login}</span>
    {:else if result.type === 'virtual'}
      <div
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-semibold text-muted"
      >
        <span class="iconify h-4 w-4 uil--megaphone"></span>
      </div>
      <span class="min-w-0 truncate text-sm text-text">{result.label}</span>
      <span class="min-w-0 truncate text-sm text-muted">@{result.handle}</span>
    {:else}
      <div
        class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-semibold text-muted"
      >
        <span class="iconify h-4 w-4 uil--users-alt"></span>
      </div>
      <span class="min-w-0 truncate text-sm text-text">{m['composer.mention.role']()}</span>
      <span class="min-w-0 truncate text-sm text-muted">@{result.role.name}</span>
    {/if}
  {/snippet}
</AutocompletePopup>
