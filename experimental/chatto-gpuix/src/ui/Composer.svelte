<script lang="ts">
  import { inputTheme } from './theme.ts';
  import ActionButton from '../ActionButton.svelte';
  let {
    idPrefix = 'live',
    placeholder = 'Type a message…',
    label = 'Message',
    draft,
    sending,
    canSend,
    onChange,
    onSend
  }: {
    idPrefix?: string;
    placeholder?: string;
    label?: string;
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
    {placeholder}
    aria-label={label}
    onchange={(event) => onChange(event.value)}
    onsubmit={onSend}
    testid={`${idPrefix}-draft`}
  />
  <ActionButton
    label={sending ? 'Sending…' : 'Send'}
    icon="send"
    quiet
    disabled={!canSend || sending}
    onclick={onSend}
    testid={`${idPrefix}-send`}
  />
</div>

<style>
  .composer {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-8);
    padding: var(--size-4) var(--size-8) var(--size-4) var(--size-12);
    margin: var(--size-8) var(--size-10) var(--size-10);
    background-color: var(--color-surface);
    border-width: var(--size-1);
    border-color: #333333;
    border-radius: var(--radius-panel);
  }
  .format {
    font-size: var(--text-body);
    font-weight: 600;
    color: var(--color-muted);
  }
  .draft {
    font-family: var(--font-sans);
    font-size: var(--text-body);
    flex-grow: 1;
    min-width: 0;
    height: var(--size-38);
  }
</style>
