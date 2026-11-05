<script lang="ts">
  import type { NativeSession } from './session.svelte.ts';
  import ScaleControls from './ScaleControls.svelte';
  import Avatar from './ui/Avatar.svelte';
  import ActionButton from './ActionButton.svelte';
  let { session }: { session: NativeSession } = $props();
</script>

<div class="manager">
  <text class="title">Settings</text>
  <ScaleControls {session} />
  <text class="name">Your servers</text>
  <text class="muted">Connections and sign-in sessions are saved securely on this Mac.</text>
  {#if session.error}<text class="error">{session.error}</text>{/if}
  {#each session.servers as record, index (record.id)}
    <div class="row">
      <Avatar name={record.name} src={record.iconUrl} serverUrl={record.url} square />
      <div class="description">
        <text class="name">{record.name}</text><text class="muted">{new URL(record.url).host}</text
        ><text class="muted"
          >{record.token && record.reauthRequiredAt === null
            ? 'Signed in'
            : 'Sign-in required'}</text
        >
      </div>
      <ActionButton label="Open" quiet onclick={() => session.activate(record.id)} />
      <ActionButton
        label="Up"
        quiet
        disabled={index === 0}
        onclick={() => session.moveServer(record.id, -1)}
      />
      <ActionButton
        label="Down"
        quiet
        disabled={index === session.servers.length - 1}
        onclick={() => session.moveServer(record.id, 1)}
      />
      <ActionButton
        label="Remove"
        quiet
        onclick={() => void session.removeServer(record.id)}
        testid={`remove-${record.id}`}
      />
    </div>
  {/each}
  {#if !session.servers.length}<text class="muted">Add a server to get started.</text>{/if}
  <ActionButton label="Add server" icon="plus" primary onclick={() => session.beginAdd()} />
</div>

<style>
  .manager {
    display: flex;
    flex-direction: column;
    gap: var(--size-14);
    padding: var(--size-28);
    width: 100%;
    height: 100%;
    overflow-y: scroll;
  }
  .title {
    font-size: var(--text-title);
    font-weight: 600;
    color: var(--color-text-top);
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .error {
    font-size: var(--text-small);
    color: var(--color-error);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--size-12);
    padding: var(--size-12);
    background-color: var(--color-surface);
    border-radius: var(--size-10);
  }
  .description {
    display: flex;
    flex-direction: column;
    gap: var(--size-3);
    flex-grow: 1;
    min-width: 0;
  }
  .name {
    font-size: var(--text-body);
    font-weight: 600;
  }
</style>
