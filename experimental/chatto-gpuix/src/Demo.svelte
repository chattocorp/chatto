<script lang="ts">
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
      <div class="brand">
        <div class="brand-row">
          <text class="brand-name">Chatto</text><text class="experiment">02</text>
        </div>
        <text class="muted">Native lab</text>
      </div>
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
      <div class="header">
        <div class="room-heading">
          <text class="room-title" testid="room-title"># {room.name}</text>
          <text class="muted">{room.description}</text>
        </div>
        <div class="local-badge"><text>Local demo</text></div>
      </div>
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
    width: 250px;
    height: 100%;
    flex-shrink: 0;
    background-color: var(--color-background);
  }
  .brand {
    display: flex;
    flex-direction: column;
    gap: 7px;
    padding: 24px 20px;
  }
  .brand-row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 10px;
  }
  .brand-name {
    font-size: 23px;
    font-weight: bold;
    color: var(--color-text-top);
  }
  .experiment {
    font-size: 11px;
    color: var(--color-action);
    background-color: var(--color-selected);
    padding: 3px 7px;
    border-radius: 5px;
  }
  .muted {
    font-size: 12px;
    color: var(--color-muted);
  }
  .channels {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 16px 10px;
    flex-grow: 1;
  }
  .section-label {
    font-size: 10px;
    font-weight: bold;
    color: var(--color-muted);
    padding: 0 12px 10px;
  }
  .room {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    border-radius: 7px;
    font-size: 14px;
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
    font-size: 19px;
  }
  .sidebar-footer {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 20px;
  }
  .footer-title {
    font-size: 12px;
    color: var(--color-text);
  }
  .conversation {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
    height: 100%;
  }
  .header {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 16px;
    padding: 20px 24px;
    border-bottom-width: 1px;
    border-color: var(--color-border);
  }
  .room-heading {
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-grow: 1;
  }
  .room-title {
    font-size: 19px;
    font-weight: bold;
    color: var(--color-text-top);
  }
  .local-badge {
    padding: 5px 9px;
    background-color: #273b32;
    color: #a8ceb7;
    font-size: 11px;
    border-radius: 6px;
  }
  .toolbar {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 12px;
    padding: 12px 24px;
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
    gap: 9px;
    padding: 14px 24px 18px;
  }
  .composer {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border-radius: 10px;
    background-color: var(--color-surface);
  }
  .draft {
    flex-grow: 1;
    min-width: 0;
    height: 35px;
    font-size: 14px;
    color: var(--color-text);
  }
  .composer-note {
    font-size: 11px;
    color: var(--color-muted);
  }
</style>
