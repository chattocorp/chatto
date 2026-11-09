<script lang="ts">
  import type { NativeSession } from './session.svelte.ts';
  import Avatar from './ui/Avatar.svelte';
  import IconButton from './ui/IconButton.svelte';
  let { session }: { session: NativeSession } = $props();
</script>

<div class="gutter">
  <div class="servers">
    {#each session.servers as record (record.id)}
      {@const selected =
        session.activeServerId === record.id && !session.adding && !session.managing}
      {@const unread = record.token
        ? Object.values(
            session.serverFor(record.id)?.notifications.roomImportantUnreadCounts ?? {}
          ).reduce((sum, count) => sum + count, 0)
        : 0}
      <div
        class={['server', { selected }]}
        role="button"
        aria-label={`${record.name}, ${new URL(record.url).host}${!record.token || record.reauthRequiredAt !== null ? ', sign-in required' : ''}`}
        tabindex="0"
        hitbox="self"
        testid={`server-${record.id}`}
        onclick={() => session.activate(record.id)}
        onkeydown={(event) => {
          if (event.key === 'enter' || event.key === 'space') session.activate(record.id);
        }}
      >
        <Avatar name={record.name} src={record.iconUrl} serverUrl={record.url} size={36} square />
        {#if !record.token || record.reauthRequiredAt !== null}<text class="reauth">!</text>
        {:else if unread}<text class="unread">{unread > 99 ? '99+' : unread}</text>{/if}
      </div>
    {/each}
    <IconButton
      label="Add server"
      icon="plus"
      onclick={() => session.beginAdd()}
      disabled={session.booting}
      testid="add-server"
    />
  </div>
  <IconButton
    label="Settings and servers"
    icon="settings"
    onclick={() => {
      session.cancelSignIn();
      session.managing = !session.managing;
      session.adding = false;
    }}
    testid="manage-servers"
    disabled={session.booting}
  />
</div>

<style>
  .gutter {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: var(--size-56);
    flex-shrink: 0;
    height: 100%;
    padding: var(--size-8) var(--size-5);
    border-right-width: var(--size-1);
    border-color: var(--color-border);
  }
  .servers {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--size-7);
    flex-grow: 1;
    min-height: 0;
    overflow-y: scroll;
  }
  .server {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--size-3);
    border-radius: var(--size-13);
    border-width: var(--size-1);
    border-color: transparent;
    cursor: pointer;
  }
  .server:hover {
    background-color: var(--color-surface);
  }
  .server.selected {
    background-color: var(--color-selected);
    border-color: var(--color-action);
  }
  .server:focus-visible {
    border-color: var(--color-action);
  }
  .unread {
    font-size: var(--text-badge);
    color: #05232c;
    background-color: var(--color-action);
    border-radius: var(--size-7);
    padding: var(--size-1) var(--size-3);
  }
  .reauth {
    font-size: var(--text-small);
    color: var(--color-error);
    padding: 0 var(--size-2);
  }
</style>
