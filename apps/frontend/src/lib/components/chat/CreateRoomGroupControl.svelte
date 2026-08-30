<!--
@component

Inline server-sidebar control for creating a room group without leaving chat.
The compact form expands in place and submits with Enter.
-->
<script lang="ts">
  import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  const roomLayoutAPI = serverScope.connection.getAPI(createAdminRoomLayoutAPI);

  let expanded = $state(false);
  let name = $state('');
  let saving = $state(false);

  function open(): void {
    name = '';
    expanded = true;
  }

  function close(): void {
    if (saving) return;
    expanded = false;
    name = '';
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || saving) return;
    saving = true;
    try {
      await roomLayoutAPI.createRoomGroup({ name: trimmedName });
      if (!serverScope.isCurrent()) return;
      toast.success(m('admin.rooms_admin.group_created'));
      expanded = false;
      name = '';
    } catch (error) {
      toast.error(
        m('admin.rooms_admin.create_group_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    } finally {
      saving = false;
    }
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Escape') return;
    event.preventDefault();
    close();
  }
</script>

<div class="px-2 pb-2" data-testid="create-room-group-control">
  {#if expanded}
    <form class="flex items-end gap-1" onsubmit={submit}>
      <div class="min-w-0 flex-1">
        <TextInput
          id="sidebar-new-room-group"
          label={m('admin.rooms_admin.group_name')}
          labelHidden
          placeholder={m('admin.rooms_admin.group_name_placeholder')}
          bind:value={name}
          disabled={saving}
          autofocus
          onkeydown={handleKeydown}
        />
      </div>
      <button
        type="submit"
        class="icon-action h-9 w-9"
        aria-label={m('admin.rooms_admin.create_group')}
        disabled={!name.trim() || saving}
      >
        <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
      </button>
      <button
        type="button"
        class="icon-action h-9 w-9"
        aria-label={m('common.cancel')}
        disabled={saving}
        onclick={close}
      >
        <span class="iconify icon-[uil--times]" aria-hidden="true"></span>
      </button>
    </form>
  {:else}
    <button type="button" class="sidebar-item text-muted" onclick={open}>
      <span class="iconify sidebar-icon icon-[uil--plus]" aria-hidden="true"></span>
      {m('admin.rooms_admin.new_group')}
    </button>
  {/if}
</div>
