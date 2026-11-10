<!-- @component
Search known servers and their visible channel rooms. The editor keeps focus
and forwards keys; selecting a result inserts its owning server's share URL.
-->
<script lang="ts">
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import type { RoomLinkSource } from './roomLinkSources';
  import { fuzzyMatch } from '$lib/fuzzyMatch';
  import { roomIcon } from '$lib/render/roomIcon';
  import AutocompletePopup from './AutocompletePopup.svelte';

  let {
    query,
    sources,
    onSelect,
    onClose
  }: {
    query: string;
    sources: readonly RoomLinkSource[];
    onSelect: (url: string, key: string) => void;
    onClose: () => void;
  } = $props();

  const results = $derived(
    sources
      .flatMap((source) => [
        { source, room: null, name: source.name, url: new URL('/chat/-/', source.origin).href },
        ...source.rooms
          .filter((room) => room.type === RoomKind.CHANNEL)
          .map((room) => ({
            source,
            room,
            name: room.name,
            url: new URL(`/chat/-/${encodeURIComponent(room.id)}`, source.origin).href
          }))
      ])
      .map((item) => ({
        ...item,
        score: query
          ? Math.max(
              fuzzyMatch(query, item.name) ?? 0,
              fuzzyMatch(query, item.room ? item.source.name : item.source.origin) ?? 0
            )
          : 1
      }))
      .filter(({ score }) => score > 0)
      .sort(
        (a, b) => b.score - a.score || a.name.localeCompare(b.name) || a.url.localeCompare(b.url)
      )
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
  getKey={({ url }) => url}
  onSelect={({ url }, key) => onSelect(url, key)}
  {onClose}
  testid="room-autocomplete"
>
  {#snippet item({ item: { room, source, name } })}
    {@const icon = room ? roomIcon(room) : { className: 'icon-[uil--server]', text: '' }}
    <span aria-hidden="true" class="flex h-6 w-6 shrink-0 items-center justify-center text-muted">
      <span class={icon.className}>{icon.text}</span>
    </span>
    <bdi class="min-w-0 truncate text-sm text-text">{name}</bdi>
    <bdi class="ms-auto min-w-0 truncate text-sm text-muted"
      >{room ? source.name : new URL(source.origin).host}</bdi
    >
  {/snippet}
</AutocompletePopup>
