<script lang="ts">
  import { Button } from '$lib/ui/form';
  import Dialog from '$lib/ui/Dialog.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    roleDisplayName,
    deleting = false,
    onConfirm,
    onCancel
  }: {
    roleDisplayName: string;
    deleting?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();

  let visible = $state(true);

  function handleClose() {
    visible = false;
    onCancel();
  }
</script>

<Dialog {visible} title={m['rbac.delete_role.title']()} size="sm" onclose={handleClose}>
  <p class="mb-4 text-muted">
    {m['rbac.delete_role.prompt']({ role: roleDisplayName })}
  </p>
  <ul class="mb-4 list-inside list-disc text-sm text-muted">
    <li>{m['rbac.delete_role.remove_from_users']()}</li>
    <li>{m['rbac.delete_role.delete_grants']()}</li>
  </ul>
  <p class="text-sm font-medium text-error">{m['rbac.delete_role.irreversible']()}</p>

  {#snippet footer()}
    <div class="flex justify-end gap-3">
      <Button variant="secondary" onclick={handleClose} disabled={deleting}
        >{m['common.cancel']()}</Button
      >
      <Button variant="danger" onclick={onConfirm} disabled={deleting}>
        {deleting ? m['rbac.delete_role.deleting']() : m['rbac.delete_role.action']()}
      </Button>
    </div>
  {/snippet}
</Dialog>
