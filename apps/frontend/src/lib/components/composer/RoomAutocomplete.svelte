<!-- @component
Search the current server's visible channel rooms. The editor keeps focus and
forwards keys; selecting a room inserts its existing share URL.
-->
<script lang="ts">
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import { fuzzyMatch } from '$lib/fuzzyMatch';
  import { roomIcon } from '$lib/render/roomIcon';
  import AutocompletePopup from './AutocompletePopup.svelte';

  let {
    query,
    rooms,
    onSelect,
    onClose
  }: {
    query: string;
    rooms: readonly RoomListItem[];
    onSelect: (roomId: string, key: string) => void;
    onClose: () => void;
  } = $props();

  const results = $derived(
    rooms
      .filter((room) => room.type === RoomKind.CHANNEL)
      .map((room) => ({ room, score: query ? (fuzzyMatch(query, room.name) ?? 0) : 1 }))
      .filter(({ score }) => score > 0)
      .sort(
        (a, b) =>
          b.score - a.score ||
          a.room.name.localeCompare(b.room.name) ||
          a.room.id.localeCompare(b.room.id)
      )
      .slice(0, 10)
  );
  let popup = $state<{ handleKeyDown: (event: KeyboardEvent) => boolean } | null>(null);

  /** Forward editor keys to the shared popup without moving focus. */
  export function handleKeyDown(event: KeyboardEvent): boolean {
    return popup?.handleKeyDown(event) ?? false;
  }
</script>

<AutocompletePopup
  bind:this={popup}
  items={results}
  getKey={({ room }) => room.id}
  onSelect={({ room }, key) => onSelect(room.id, key)}
  {onClose}
  testid="room-autocomplete"
>
  {#snippet item({ item: { room } })}
    {@const icon = roomIcon(room)}
    <span aria-hidden="true" class="flex h-6 w-6 shrink-0 items-center justify-center text-muted">
      <span class={icon.className}>{icon.text}</span>
    </span>
    <bdi class="min-w-0 truncate text-sm text-text">{room.name}</bdi>
  {/snippet}
</AutocompletePopup>
