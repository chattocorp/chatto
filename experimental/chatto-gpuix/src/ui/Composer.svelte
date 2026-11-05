<script lang="ts">
  import { inputTheme } from './theme.ts';
  import ActionButton from '../ActionButton.svelte';
  let {
    draft,
    sending,
    canSend,
    onChange,
    onSend
  }: {
    draft: string;
    sending: boolean;
    canSend: boolean;
    onChange: (value: string) => void;
    onSend: () => void;
  } = $props();
</script>

<div class="composer">
  <text class="format">Aa</text>
  <input
    class="draft"
    value={draft}
    theme={inputTheme}
    placeholder="Type a message…"
    aria-label="Message"
    onchange={(event) => onChange(event.value)}
    onsubmit={onSend}
    testid="live-draft"
  />
  <ActionButton
    label={sending ? 'Sending…' : 'Send'}
    icon="send"
    quiet
    disabled={!canSend || sending}
    onclick={onSend}
    testid="live-send"
  />
</div>

<style>
  .composer {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding: 4px 8px 4px 12px;
    margin: 8px 10px 10px;
    background-color: var(--color-surface);
    border-width: 1px;
    border-color: #333333;
    border-radius: var(--radius-panel);
  }
  .format {
    font-size: 13px;
    font-weight: 600;
    color: var(--color-muted);
  }
  .draft {
    flex-grow: 1;
    min-width: 0;
    height: 38px;
  }
</style>
