<script lang="ts">
  import PaneHeader from './ui/PaneHeader.svelte';
  import Theme from './ui/Theme.svelte';
  import { inputTheme } from './ui/theme.ts';
  import type { GpuixEvent } from 'gpuix-svelte';
  import ActionButton from './ActionButton.svelte';
  import MessageRow from './MessageRow.svelte';
  import { initialHistories, maxMessages, rooms, sampleMessages, type RoomId } from './demo.ts';

  let selected = $state<RoomId>('general');
  let histories = $state.raw(initialHistories());
  let drafts = $state<Record<RoomId, string>>({ general: '', rendering: '', playground: '' });
  let localSequence = 0;
  const room = $derived(rooms.find((room) => room.id === selected)!);
  const messages = $derived(histories[selected]);
  const canAdd = $derived(messages.length < maxMessages);
  const canSend = $derived(canAdd && drafts[selected].trim().length > 0);

  function choose(id: RoomId) {
    selected = id;
  }
  function chooseWithKey(event: GpuixEvent, id: RoomId) {
    if (event.key === 'enter' || event.key === 'space') choose(id);
  }
  function addSamples() {
    if (!canAdd) return;
    histories = {
      ...histories,
      [selected]: [
        ...messages,
        ...sampleMessages(selected, Math.min(100, maxMessages - messages.length), messages.length)
      ]
    };
  }
  function send() {
    const body = drafts[selected].trim();
    if (!body || !canAdd) return;
    histories = {
      ...histories,
      [selected]: [
        ...messages,
        {
          id: `${selected}:local:${++localSequence}`,
          author: 'You (local)',
          time: new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(
            new Date()
          ),
          body,
          format: 'text'
        }
      ]
    };
    drafts[selected] = '';
  }
</script>

<Theme>
  <div class="app">
    <div class="sidebar">
      <PaneHeader title="Chatto" description="Native lab" />
      <div class="channels" role="tablist" aria-label="Channels">
        <text class="section-label">CHANNELS</text>
        {#each rooms as item (item.id)}
          <div
            class={['room', { selected: selected === item.id }]}
            role="tab"
            aria-label={`Open ${item.name}`}
            aria-selected={selected === item.id}
            tabindex="0"
            hitbox="self"
            testid={`room-${item.id}`}
            onclick={() => choose(item.id)}
            onkeydown={(event) => chooseWithKey(event, item.id)}
          >
            <text class="hash">#</text><text>{item.name}</text>
          </div>
        {/each}
      </div>
      <div class="sidebar-footer">
        <text class="footer-title">A different way to build native.</text>
        <text class="muted">Svelte + GPUix</text>
      </div>
    </div>
    <div class="conversation">
      <PaneHeader title={room.name} description={room.description} icon="hash">
        {#snippet actions()}<div class="local-badge"><text>Local demo</text></div>{/snippet}
      </PaneHeader>
      <div class="toolbar">
        <text class="muted" testid="message-count">{messages.length} messages</text>
        <div class="spacer"></div>
        <ActionButton
          label="Add 100 samples"
          testid="add-samples"
          onclick={addSamples}
          disabled={!canAdd}
        />
      </div>
      <!-- A room change is a history boundary, so give it a fresh native scroll anchor. -->
      {#key selected}
        <virtual-list
          class="timeline"
          alignment="bottom"
          followTail={true}
          overdraw={240}
          estimatedItemHeight={120}
          testId="timeline"
        >
          {#each messages as message (message.id)}
            <MessageRow {message} />
          {/each}
        </virtual-list>
      {/key}
      <div class="composer-area">
        <div class="composer">
          <input
            class="draft"
            theme={{
              fontSans: 'IBM Plex Sans',
              fontMono: 'IBM Plex Mono',
              appearance: 'dark',
              bg: 'var(--color-surface)',
              text: 'var(--color-text)',
              accent: '#8171bc'
            }}
            value={drafts[selected]}
            placeholder={`Message #${room.name} (local only)`}
            aria-label={`Local message in ${room.name}`}
            testid="draft"
            onchange={(event) => (drafts[selected] = event.value)}
            onsubmit={send}
          />
          <ActionButton
            label="Add message"
            primary
            testid="send"
            onclick={send}
            disabled={!canSend}
          />
        </div>
        <text class="composer-note"
          >Sample data. Nothing is sent or saved when you close the app.</text
        >
      </div>
    </div>
  </div>
</Theme>

<style>
  .app {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    background-color: var(--color-background);
    color: var(--color-text);
  }
  .sidebar {
    display: flex;
    flex-direction: column;
    width: var(--size-250);
    height: 100%;
    flex-shrink: 0;
    background-color: var(--color-background);
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .channels {
    display: flex;
    flex-direction: column;
    gap: var(--size-4);
    padding: var(--size-16) var(--size-10);
    flex-grow: 1;
  }
  .section-label {
    font-size: var(--text-badge);
    font-weight: bold;
    color: var(--color-muted);
    padding: 0 var(--size-12) var(--size-10);
  }
  .room {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-10);
    padding: var(--size-10) var(--size-14);
    border-radius: var(--size-7);
    font-size: var(--text-body);
    color: var(--color-text);
    cursor: pointer;
  }
  .room:hover {
    background-color: var(--color-surface);
  }
  .room.selected {
    background-color: var(--color-selected);
    color: var(--color-text-top);
  }
  .hash {
    color: var(--color-muted);
    font-size: var(--text-title);
  }
  .sidebar-footer {
    display: flex;
    flex-direction: column;
    gap: var(--size-8);
    padding: var(--size-20);
  }
  .footer-title {
    font-size: var(--text-small);
    color: var(--color-text);
  }
  .conversation {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
    height: 100%;
  }
  .local-badge {
    padding: var(--size-5) var(--size-9);
    background-color: #273b32;
    color: #a8ceb7;
    font-size: var(--text-small);
    border-radius: var(--size-6);
  }
  .toolbar {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-12);
    padding: var(--size-12) var(--size-24);
  }
  .spacer {
    flex-grow: 1;
  }
  .timeline {
    flex-grow: 1;
    min-height: 0;
    width: 100%;
  }
  .composer-area {
    display: flex;
    flex-direction: column;
    gap: var(--size-9);
    padding: var(--size-14) var(--size-24) var(--size-18);
  }
  .composer {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-12);
    padding: var(--size-9) var(--size-12);
    border-radius: var(--size-10);
    background-color: var(--color-surface);
  }
  .draft {
    font-family: var(--font-sans);
    font-size: var(--text-body);
    flex-grow: 1;
    min-width: 0;
    height: var(--size-35);
    font-size: var(--text-body);
    color: var(--color-text);
  }
  .composer-note {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
</style>
